// Licensed under the MIT license.
// Inspired by the kured project: https://github.com/weaveworks/kured
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackfrancis/kustodian/pkg/basicserver"
	mylog "github.com/jackfrancis/kustodian/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/kured/pkg/daemonsetlock"
	"github.com/weaveworks/kured/pkg/delaytick"
	"github.com/weaveworks/kured/pkg/taints"
	"github.com/weaveworks/kured/pkg/timewindow"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kubectldrain "k8s.io/kubectl/pkg/drain"
)

var (
	version = "unreleased"

	// Command line flags
	period                    time.Duration
	dsNamespace               string
	dsName                    string
	lockAnnotation            string
	lockTTL                   time.Duration
	maintenanceSentinel       string
	preferNoScheduleTaintName string
	podSelectors              []string

	maintenanceDays  []string
	maintenanceStart string
	maintenanceEnd   string
	timezone         string
	annotateNodes    bool

	metricserver            *basicserver.BasicServer
	nodeInMaintenanceWindow prometheus.Gauge
	maintenanceInProgress   prometheus.Gauge
	maintenanceCompleted    prometheus.Counter
)

const (
	// KustodianMaintenanceRequiredSentinelFilePath is the canonical filepath for indicating that node maintenance is required
	KustodianMaintenanceRequiredSentinelFilePath string = "/var/maintenance-required"
	// KustodianMaintenanceInProgressSentinelFilePath is the canonical filepath for indicating that active node maintenance is in progress
	KustodianMaintenanceInProgressSentinelFilePath string = "/var/maintenance-in-progress"
	// KustodianNodeLockAnnotation is the canonical string value for the kustodian node-lock annotation
	KustodianNodeLockAnnotation string = "k8s.io/kustodian-node-lock"
	// KustodianMaintenanceInProgressAnnotation is the canonical string value for the maintenance-in-progress annotation
	KustodianMaintenanceInProgressAnnotation string = "k8s.io/maintenance-in-progress"
	// KustodianMostRecentMaintenanceNeededAnnotation is the canonical string value for the most-recent-maintenance-needed annotation
	KustodianMostRecentMaintenanceNeededAnnotation string = "k8s.io/most-recent-maintenance-needed"
	// prefix for metrics
	KustodianMetricPrefix string = "kustodian"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kustodian",
		Short: "Kubernetes Node Maintainer",
		Run:   root}

	rootCmd.PersistentFlags().DurationVar(&period, "period", time.Minute*60,
		"maintenance check period")
	rootCmd.PersistentFlags().StringVar(&dsNamespace, "ds-namespace", "kube-system",
		"namespace containing daemonset on which to place lock")
	rootCmd.PersistentFlags().StringVar(&dsName, "ds-name", "kustodian",
		"name of daemonset on which to place lock")
	rootCmd.PersistentFlags().StringVar(&maintenanceSentinel, "maintenance-sentinel", KustodianMaintenanceRequiredSentinelFilePath,
		"path to file whose existence signals that maintenance is needed")
	rootCmd.PersistentFlags().StringVar(&preferNoScheduleTaintName, "prefer-no-schedule-taint", "",
		"Taint name applied during pending node reboot (to prevent receiving additional pods from other rebooting nodes). Disabled by default. Set e.g. to \"kustodian.maintenance\" to enable tainting.")
	rootCmd.PersistentFlags().StringVar(&lockAnnotation, "lock-annotation", KustodianNodeLockAnnotation,
		"annotation in which to record locking node")
	rootCmd.PersistentFlags().DurationVar(&lockTTL, "lock-ttl", 0,
		"expire lock annotation after this duration (default: 0, disabled)")

	rootCmd.PersistentFlags().StringSliceVar(&maintenanceDays, "maintenance-days", timewindow.EveryDay,
		"schedule maintenance on these days")
	rootCmd.PersistentFlags().StringVar(&maintenanceStart, "start-time", "0:00",
		"schedule maintenance only after this time of day")
	rootCmd.PersistentFlags().StringVar(&maintenanceEnd, "end-time", "23:59:59",
		"schedule maintenance only before this time of day")
	rootCmd.PersistentFlags().StringVar(&timezone, "time-zone", "UTC",
		"use this timezone for schedule inputs")

	rootCmd.PersistentFlags().BoolVar(&annotateNodes, "annotate-nodes", false,
		"enable 'k8s.io/maintenance-in-progress' and 'k8s.io/most-recent-maintenance-needed' node annotations to signify kustodian maintenance operations")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

// newCommand creates a new Command with stdout/stderr wired to our standard logger
func newCommand(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)

	cmd.Stdout = log.NewEntry(log.StandardLogger()).
		WithField("cmd", cmd.Args[0]).
		WithField("std", "out").
		WriterLevel(log.InfoLevel)

	cmd.Stderr = log.NewEntry(log.StandardLogger()).
		WithField("cmd", cmd.Args[0]).
		WithField("std", "err").
		WriterLevel(log.WarnLevel)

	return cmd
}

func sentinelExists() bool {
	// Relies on hostPID:true and privileged:true to enter host mount space
	sentinelCmd := newCommand("/usr/bin/test", "-f", maintenanceSentinel)
	if err := sentinelCmd.Run(); err != nil {
		switch err := err.(type) {
		case *exec.ExitError:
			return false
		default:
			log.Fatalf("Error invoking sentinel command: %v", err)
		}
	}
	return true
}

func maintenanceRequired() bool {
	if sentinelExists() {
		log.Infof("Maintenance required")
		return true
	}
	log.Infof("Maintenance not required")
	return false
}

func holding(lock *daemonsetlock.DaemonSetLock, metadata interface{}) bool {
	holding, err := lock.Test(metadata)
	if err != nil {
		log.Fatalf("Error testing lock: %v", err)
	}
	if holding {
		log.Infof("Holding lock")
	}
	return holding
}

func acquire(lock *daemonsetlock.DaemonSetLock, metadata interface{}, TTL time.Duration) bool {
	holding, holder, err := lock.Acquire(metadata, TTL)
	switch {
	case err != nil:
		log.Fatalf("Error acquiring lock: %v", err)
		return false
	case !holding:
		log.Warnf("Lock already held: %v", holder)
		return false
	default:
		log.Infof("Acquired node maintenance lock")
		return true
	}
}

func release(lock *daemonsetlock.DaemonSetLock) {
	log.Infof("Releasing lock")
	if err := lock.Release(); err != nil {
		log.Fatalf("Error releasing lock: %v", err)
	}
}

func cordonanddrain(client *kubernetes.Clientset, node *v1.Node) {
	nodename := node.GetName()

	log.Infof("Draining node %s", nodename)

	drainer := &kubectldrain.Helper{
		Client:              client,
		GracePeriodSeconds:  -1,
		Force:               true,
		DeleteLocalData:     true,
		IgnoreAllDaemonSets: true,
		ErrOut:              os.Stderr,
		Out:                 os.Stdout,
	}
	if err := kubectldrain.RunCordonOrUncordon(drainer, node, true); err != nil {
		log.Fatalf("Error cordoning %s: %v", nodename, err)
	}

	if err := kubectldrain.RunNodeDrain(drainer, nodename); err != nil {
		log.Fatalf("Error draining %s: %v", nodename, err)
	}
}

func uncordon(client *kubernetes.Clientset, node *v1.Node) {
	nodename := node.GetName()
	log.Infof("Uncordoning node %s", nodename)
	drainer := &kubectldrain.Helper{
		Client: client,
		ErrOut: os.Stderr,
		Out:    os.Stdout,
	}
	if err := kubectldrain.RunCordonOrUncordon(drainer, node, false); err != nil {
		log.Fatalf("Error uncordoning %s: %v", nodename, err)
	}
}

func markAsMaintenanceInProgress(nodeID string) {
	log.Infof("Marking node %s for maintenance by creating %s sentinel file on host filesystem", nodeID, KustodianMaintenanceInProgressSentinelFilePath)
	maintenanceInProgressCmd := newCommand("touch", KustodianMaintenanceInProgressSentinelFilePath)
	if err := maintenanceInProgressCmd.Run(); err != nil {
		log.Fatalf("Error creating %s sentinel file on host filesystem: %v", KustodianMaintenanceInProgressSentinelFilePath, err)
	}
}

func removeMaintenanceInProgress(nodeID string) {
	log.Infof("Node maintenance is over, removing %s from node %s's host filesystem", KustodianMaintenanceInProgressSentinelFilePath, nodeID)
	maintenanceInProgressCmd := newCommand("rm", "-f", KustodianMaintenanceInProgressSentinelFilePath)
	if err := maintenanceInProgressCmd.Run(); err != nil {
		log.Fatalf("Error removing %s sentinel file from host filesystem: %v", KustodianMaintenanceInProgressSentinelFilePath, err)
	}
}

// nodeMeta is used to remember information across reboots
type nodeMeta struct {
	Unschedulable bool `json:"unschedulable"`
}

func addNodeAnnotations(client *kubernetes.Clientset, nodeID string, annotations map[string]string) {
	node, err := client.CoreV1().Nodes().Get(context.TODO(), nodeID, metav1.GetOptions{})
	if err != nil {
		log.Fatal("Error retrieving node object via k8s API: %v", err)
	}
	for k, v := range annotations {
		node.Annotations[k] = v
		log.Infof("Adding node %s annotation: %s=%s", node.GetName(), k, v)
	}

	bytes, err := json.Marshal(node)
	if err != nil {
		log.Fatal("Error marshalling node object into JSON: %v", err)
	}

	_, err = client.CoreV1().Nodes().Patch(context.TODO(), node.GetName(), types.StrategicMergePatchType, bytes, metav1.PatchOptions{})
	if err != nil {
		var annotationsErr string
		for k, v := range annotations {
			annotationsErr += fmt.Sprintf("%s=%s ", k, v)
		}
		log.Fatal("Error adding node annotations %s via k8s API: %v", annotationsErr, err)
	}
}

func deleteNodeAnnotation(client *kubernetes.Clientset, nodeID, key string) {
	log.Infof("Deleting node %s annotation %s", nodeID, key)

	// JSON Patch takes as path input a JSON Pointer, defined in RFC6901
	// So we replace all instances of "/" with "~1" as per:
	// https://tools.ietf.org/html/rfc6901#section-3
	patch := []byte(fmt.Sprintf("[{\"op\":\"remove\",\"path\":\"/metadata/annotations/%s\"}]", strings.ReplaceAll(key, "/", "~1")))
	_, err := client.CoreV1().Nodes().Patch(context.TODO(), nodeID, types.JSONPatchType, patch, metav1.PatchOptions{})
	if err != nil {
		log.Fatal("Error deleting node annotation %s via k8s API: %v", key, err)
	}
}

func cordonAndDrainAsRequired(nodeID string, window *timewindow.TimeWindow, TTL time.Duration) {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal(err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	lock := daemonsetlock.New(client, nodeID, dsNamespace, dsName, lockAnnotation)

	nodeMeta := nodeMeta{}
	if holding(lock, &nodeMeta) {
		node, err := client.CoreV1().Nodes().Get(context.TODO(), nodeID, metav1.GetOptions{})
		if err != nil {
			log.Fatal("Error retrieving node object via k8s API: %v", err)
		}
		if !maintenanceRequired() {
			uncordon(client, node)
			release(lock)
			if annotateNodes {
				if _, ok := node.Annotations[KustodianMaintenanceInProgressAnnotation]; ok {
					deleteNodeAnnotation(client, nodeID, KustodianMaintenanceInProgressAnnotation)
				}
			}
		}
	}

	var preferNoScheduleTaint *taints.Taint
	if preferNoScheduleTaintName != "" {
		preferNoScheduleTaint = taints.New(client, nodeID, preferNoScheduleTaintName, v1.TaintEffectPreferNoSchedule)
	}

	// Remove taint immediately during startup to quickly allow scheduling again.
	if preferNoScheduleTaint != nil && !maintenanceRequired() {
		preferNoScheduleTaint.Disable()
	}

	source := rand.NewSource(time.Now().UnixNano())
	tick := delaytick.New(source, period)
	for range tick {
		if !window.Contains(time.Now()) {
			// Remove taint outside the maintenance time window to allow for normal operation.
			if preferNoScheduleTaint != nil {
				preferNoScheduleTaint.Disable()
			}
			continue
		} else {

		}
		node, err := client.CoreV1().Nodes().Get(context.TODO(), nodeID, metav1.GetOptions{})
		if err != nil {
			log.Fatal("Error retrieving node object via k8s API: %v", err)
		}

		if !maintenanceRequired() {
			if preferNoScheduleTaint != nil {
				preferNoScheduleTaint.Disable()
			}
			if holding(lock, &nodeMeta) {
				uncordon(client, node)
				if annotateNodes {
					if _, ok := node.Annotations[KustodianMaintenanceInProgressAnnotation]; ok {
						deleteNodeAnnotation(client, nodeID, KustodianMaintenanceInProgressAnnotation)
					}
				}
				removeMaintenanceInProgress(nodeID)
				release(lock)
				maintenanceInProgress.Dec()
				maintenanceCompleted.Inc()
			}
			continue
		}
		nodeMeta.Unschedulable = node.Spec.Unschedulable

		var timeNowString string
		if annotateNodes {
			if _, ok := node.Annotations[KustodianMaintenanceInProgressAnnotation]; !ok {
				timeNowString = time.Now().Format(time.RFC3339)
				// Annotate this node to indicate that "I am in an active state of maintenance!"
				// so that other node maintenance tools running on the cluster are aware that this node is in the process of a "state transition"
				annotations := map[string]string{KustodianMaintenanceInProgressAnnotation: timeNowString}
				// & annotate this node with a timestamp so that other node maintenance tools know how long it's been since this node has been marked for maintenance
				annotations[KustodianMostRecentMaintenanceNeededAnnotation] = timeNowString
				addNodeAnnotations(client, nodeID, annotations)
			}
		}

		if !acquire(lock, &nodeMeta, TTL) {
			if preferNoScheduleTaint != nil {
				// Prefer to not schedule pods onto this node to avoid draining the same pod multiple times.
				preferNoScheduleTaint.Enable()
			}
			continue
		}

		cordonanddrain(client, node)
		markAsMaintenanceInProgress(nodeID)
		maintenanceInProgress.Inc()
	}
}

func createMetrics() {
	nodeInMaintenanceWindow = promauto.NewGauge(prometheus.GaugeOpts{
		Name: KustodianMetricPrefix + "_node_in_maintenance_window",
		Help: "Whether the node is in the maintenance window",
	})
	maintenanceInProgress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: KustodianMetricPrefix + "_node_performing_maintenance",
		Help: "Whether the node is in the maintenance window",
	})
	maintenanceCompleted = promauto.NewCounter(prometheus.CounterOpts{
		Name: KustodianMetricPrefix + "_node_completed_maintenance",
		Help: "Node completed maintenance",
	})
}

func root(cmd *cobra.Command, args []string) {
	mylog.Init(logrus.WarnLevel)
	createMetrics()
	metricserver = basicserver.CreateBasicServer()
	metricserver.StartListen(basicserver.DefaultMux())
	log.Infof("kustodian: Kubernetes Node Maintenance %s", version)

	nodeID := os.Getenv("KUSTODIAN_NODE_ID")
	if nodeID == "" {
		log.Fatal("KUSTODIAN_NODE_ID environment variable required")
	}

	window, err := timewindow.New(maintenanceDays, maintenanceStart, maintenanceEnd, timezone)
	if err != nil {
		log.Fatalf("Failed to build time window: %v", err)
	}

	log.Infof("Node ID: %s", nodeID)
	log.Infof("Lock Annotation: %s/%s:%s", dsNamespace, dsName, lockAnnotation)
	if lockTTL > 0 {
		log.Infof("Lock TTL set, lock will expire after: %v", lockTTL)
	} else {
		log.Info("Lock TTL not set, lock will remain until being released")
	}
	log.Infof("PreferNoSchedule taint: %s", preferNoScheduleTaintName)
	log.Infof("Maintenance Sentinel: %s every %v", maintenanceSentinel, period)
	log.Infof("Blocking Pod Selectors: %v", podSelectors)
	log.Infof("Allow maintenance on: %v", window)
	if annotateNodes {
		log.Infof("Will annotate nodes when kustodian cordons and drains node for maintenance")
	}

	cordonAndDrainAsRequired(nodeID, window, lockTTL)
}
