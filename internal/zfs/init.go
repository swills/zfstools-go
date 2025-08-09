package zfs

import "os/exec"

var RunZfsFn = exec.Command

var runZpoolFn = exec.Command

var ListSnapshotsFn = ListSnapshots
