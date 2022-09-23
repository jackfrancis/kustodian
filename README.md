# Kustodian

The Kustodian project is a Kubernetes-native solution to the problem of safely performing in-place node maintenance tasks.

# Status of Project

The Kustodian set of tools are currently experimental.

# Purpose of Kustodian

This project is inspired by [the Kured project](https://github.com/kubereboot/kured), which is the de facto standard Kubernetes tool for automating reliable, graceful node reboots in a cluster.

Kustodian adheres to [Unix philosophical adage: "Make each program do one thing well"](https://en.wikipedia.org/wiki/Unix_philosophy). And in that spirit, inspired by Kured's _doing reboots well_, we aim to do node maintenance well.

# Architectural Overview

Kustodian proposes three primary actors to fulfill reliable, cluster-safe node maintenance:

1. An "always on" runtime on each node that monitors its own host OS for an indication that maintenance is needed. When self-maintenance is indicated, this runtime will wait to acquire an exclusive "node maintenance" lock in the cluster. This is to ensure that we aren't doing simultaneous maintenance on more than one node at a time, potentially degrading operational availability, and making it easier to triage and potentially rollback unexpected outcomes during maintenance. We expect this runtime to be present on every node in the cluster that is appropriate for emergency maintenance (e.g., "immutable" nodes are probably not candidates for emergency maintenance); and we expect that Kustodian is the exclusive maintenance interface for node maintenance on the cluster.
2. A maintenance script that runs on the node's host OS itself as root. We expect this maintenance script to be aware of other scripts running on other nodes (the canonical use case is running _the same_ script on all nodes) and to wait for permission to proceed, and to conditionally report back success for failure, in order to fulfill the exclusivity requirements of node maintenance: one node is under maintenance at a time, and the forward progess of maintenance onto the remaining nodes depends upon the successful maintenance outcome of the node before it.
3. Because it is common for node host OS maintenance to include a reboot as a condition of the total maintenance transaction, we need to ensure that the set of required Kustodian tools includes a node reboot actor that enforces equivalent exclusivity (i.e., one node at a time). We will depend up the existing Kured tool for this.

# How does it work?

## Node maintenance specification

The Kustodian daemon runs on each node in a Kubernetes cluster, and continually looks for the presence of a sentinel file on the node filesystem: `/var/maintenance-required`. When that file is detected on a node, that node's Kustodian daemon waits until it can reserve an exclusive "single node maintenance lock", after which point it will reserve that lock, and then gracefully cordon + drain that node.

After the node is successfully cordoned + drained, the Kustodian daemon then creates a new sentinel file: `/var/maintenance-in-progress`, which indicates to the node host OS that this Kubernetes node is in a maintenance state, and is not actively participating in the cluster.

At this point, Kustodian waits for the **non-existence** of the original sentinel file `/var/maintenance-required`, which indicates that node maintenance is complete. The sentinel file `/var/maintenance-in-progress` is then deleted, and the node is rejoined to the cluster via an uncordon operation.

The Kustodian daemon does the above continually on all nodes: the practical outcome is that for a given node maintenance operation meant to be performed on *all* nodes, Kustodian only executes maintenance on one node at a time, and only after that node is able to be successfully cordoned + drained.

The above follows a sort of [pub sub pattern](https://en.wikipedia.org/wiki/Publish–subscribe_pattern), using the host OS filesystem to pass messages back and forth between the Kustodian daemon, which has privileged access to the Kubernetes cluster, and a node maintenance runtime, which has privileged access to the host OS.

This decribes the behavior from the persepctive of the Kustodian daemon. To describe the behavior from the perspective of the maintenance script running on a host OS:

1. A node maintenance script must register itself for maintenance by creating the `/var/maintenance-required` sentinel file.
2. That script must then wait in a loop for the existence of a `/var/maintenance-in-progress` sentinel file, which indicates permission for the script to begin node maintenance.
3. After the maintenance script has completed, and any appropriate node health validation has been performed, the script registers its completion by deleting the `/var/maintenance-required` sentinel file.
  - If the operation of the maintenance script has failed, or produced undesired side effects, the script would purposefully *not* delete the `/var/maintenance-required` sentinel file on the host filesystem. Keeping that file around guarantees that this Kubernetes node will continue to be cordoned (not actively participating in the cluster); furthermore, it will guarantee that no other nodes will be negatively affected similarly (Kustodian's "single node maintenance lock" will continue to be reserved by this node).
4. The script must also be sensitive to the outcome of a required reboot resulting from its performed work (e.g., updating the Linux kernel). Thus, the script should be idempotent, so that it can run again successively, making forward progress continually until its goal state is achieved.

Below is how a script might look that implements the Kustodian daemonset specification:

```bash
#!/bin/bash
# if we are in an pending reboot state we don't want to begin work
# easier just to wait until after the next reboot, when we expect this script will be run again
while fuser /var/run/reboot-required >/dev/null 2>&1; do
  echo 'Reboot pending';
  sleep 30;
done;
# if another script is already running on this host following the Kustodian pattern, then wait
until [ ! -f /var/maintenance-required ] && [ ! -f /var/maintenance-in-progress ]; do
    echo "maintenance already in-progress, will wait";
    sleep 5;
done;
# request maintenance
touch /var/maintenance-required;
# wait until Kustodian indicates that this node has been cordoned + drained, and exclusive maintenance reserved
until test -f /var/maintenance-in-progress; do
    echo "waiting in the maintenance queue";
    sleep 5;
done;
# begin maintenance
# perform maintenance
# validate host health and expected outcomes
# end maintenance
rm -f /var/maintenance-required
```

## Can I experiment with this now?

Yes! There is a prototype helm chart under `helm/kustodian` which will install the Kustodian daemonset on all nodes on your cluster. For example, assuming you check out this repository (or a fork of it), and your terminal is in the working directory of the git root:

```sh
$ helm install kustodian helm/kustodian
```

In addition, there is a prototype helm chart that allows you to experiment with running maintenance scripts that can be accessed over the public internet (or at least from the network that your node host OS is running in). An example script has been provided that updates an Ubuntu-backed node via apt. For example:

```sh
helm upgrade --install mop helm/mop --set mop.targetScript=https://github.com/jackfrancis/kustodian/raw/main/examples/apt-get-upgrade.sh --set mop.name=upgrade-ubuntu
```

Hopefully the pattern is clear and you can create your own usable scripts to experiment in your own environment.
