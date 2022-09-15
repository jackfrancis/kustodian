# Kustodian

The kustodian project is a prototype solution to the problem of safely performing in-place node maintenance tasks.

# Status of Project

The kustodian set of tools are currently experimental.

Full disclaimer: the code at present is essentially a copy pasta + refactor of some of the kured project (and kustodian re-uses some packages from kured). Kured solves the problem of safely applying node reboots, which shares some solution surface area with the problem of safely performing node maintenance.

We encourage folks who want to play around with kustodian to share thoughts in the issue queue:

- https://github.com/jackfrancis/kustodian/issues

# How does it work?

## Node maintenance specification

The kustodian daemon runs on each node in a Kubernetes cluster, and continually looks for the presence of a sentinel file on the node filesystem: `/var/maintenance-required`. When that file is detected on a node, that node's kustodian daemon waits until it can reserve an exclusive "single node maintenance lock", after which point it will reserve that lock, and then gracefully cordon + drain  that node.

After the node is successfully cordoned + drained, the kustodian daemon then creates a new sentinel file: `/var/maintenance-in-progress`, which indicates to the node host OS that this Kubernetes node is in a maintenance state, and is not actively participating in the cluster.

At this point, kustodian waits for the **non-existence** of the original sentinel file `/var/maintenance-required`, which indicates that node maintenance is complete. The sentinel file `/var/maintenance-in-progress` is then deleted, and the node is rejoined to the cluster via an uncordon operation.

The kustodian daemon does the above continually on all nodes: the practical outcome is that for a given node maintenance operation meant to be performed on *all* nodes, kustodian only executes maintenance on one node at a time, and only after that node is able to be successfully cordoned + drained.

The above follows a sort of [pub sub pattern](https://en.wikipedia.org/wiki/Publish–subscribe_pattern), using the host OS filesystem to pass messages back and forth between the kustodian daemon, which has privileged access to the Kubernetes cluster, and a node maintenance runtime, which has privileged access to the host OS.

This decribes the behavior from the persepctive of the kustodian daemon. To describe the behavior from the perspective of implementing host OS maintenance across nodes in a cluster:

1. A node maintenance script must register itself for maintenance by creating the `/var/maintenance-required` sentinel file.
2. That script must then wait in a loop for the existence of a `/var/maintenance-in-progress` sentinel file, which indicates permission for the script to begin node maintenance.
3. After the maintenance script has completed, and any appropriate node health validation has been performed, the script registers its completion by deleting the `/var/maintenance-required` sentinel file.
  - If the operation of the maintenance script has failed, or produced undesired side effects, the script would purposefully *not* delete the `/var/maintenance-required` sentinel file on the host filesystem. Keeping that file around guarantees that this Kubernetes node will continue to be cordoned (not actively participating in the cluster); furthermore, it will guarantee that no other nodes will be negatively affected similarly (kustodian's "single node maintenance lock" will continue to be reserved by this node).

Below is how a script might look that implements the kustodian daemonset specification:

```bash
#!/bin/bash
# if another script is already running on this host following the kustodian pattern, then wait
until [ ! -f /var/maintenance-required ] && [ ! -f /var/maintenance-in-progress ]; do
    echo "maintenance already in-progress, will wait";
    sleep 5;
done;
# request maintenance
touch /var/maintenance-required;
# wait until kustodian indicates that this node has been cordoned + drained, and exclusive maintenance reserved
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

Yes! There is a prototype helm chart under `helm/kustodian` which will install the kustodian daemonset on all nodes on your cluster. For example, assuming you check out this repository (or a fork of it), and your terminal is in the working directory of the git root:

```sh
$ helm upgrade --install kustodian helm/kustodian
```

In addition, there is a prototype helm chart that allows you to experiement with running maintenance scripts that can be accessed over the public internet (or at least from the network that your node host OS is running in). An example script has been provided that updates the docker (moby) container runtime assuming a systemd-enforced node host OS (e.g, Ubuntu). For example:

```sh
helm upgrade --install mop helm/mop --set mop.targetScript=https://raw.githubusercontent.com/jackfrancis/kustodian/main/examples/update-docker.sh --set mop.name=upgrade-docker
```

Again, the `update-docker.sh` script example above is highly domain specific to my experiments using self-managed Kubernetes clusters on Azure. Hopefully the pattern is clear and you can create your own usable scripts to experiment in your own environment.

# Documentation

TODO
