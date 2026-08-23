# zfstools-go
[![Go](https://github.com/swills/zfstools-go/actions/workflows/build.yml/badge.svg)](https://github.com/swills/zfstools-go/actions/workflows/build.yml)
[![golangci-lint](https://github.com/swills/zfstools-go/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/swills/zfstools-go/actions/workflows/golangci-lint.yml)

**zfstools-go** is a Go reimplementation of the
[zfstools Ruby project](https://github.com/bdrewery/zfstools). It preserves the
Ruby command model and core policy while deliberately improving validation,
error propagation, retention safety, cancellation, and dry-run reporting.

This toolkit provides automated ZFS snapshot management using three command names:

- `zfs-auto-snapshot`
- `zfs-cleanup-snapshots`
- `zfs-snapshot-mysql`

The commands can be hard links to one multi-call binary, which selects its
behavior from the invoked basename. This is not bug-for-bug compatibility with
Ruby and is not a drop-in replacement for other commands named
`zfs-auto-snapshot`.

---

## Features

- Designed for FreeBSD's `zfs` and `zpool` CLI utilities
- Automated recursive and interval-based snapshot creation
- Dry-run and verbose modes for safe operation
- Intelligent pruning of expired or zero-sized snapshots
- Optional MySQL and PostgreSQL snapshot coordination through the general
  `com.sun:auto-snapshot` property

## Lineage and Related Projects

- [Sun ZFS Automatic Snapshot](https://github.com/aszeszo/zfs-auto-snapshot):
  the historical OpenSolaris SMF implementation.
- [OpenIndiana Time Slider](https://github.com/OpenIndiana/time-slider): the
  OpenSolaris successor, with a scheduler daemon, replication, and desktop
  integration.
- [Ruby zfstools](https://github.com/bdrewery/zfstools) and the
  [maintained fork](https://github.com/swills/zfstools): the direct ancestry of
  this Go implementation.
- [ZFS-on-Linux zfs-auto-snapshot](https://github.com/zfsonlinux/zfs-auto-snapshot):
  an independent shell rewrite with an incompatible command line and policy.

See [Implementation Lineage and Design Decisions](IMPLEMENTATIONS.md) for the
detailed comparison and the policies selected by this project. The focused
Linux shell comparison is in
[ZFS-on-Linux Shell Rewrite Evaluation](shell-rewrite-compat-eval.md).

---

## Installation

Build:

```sh
go build -o zfs-auto-snapshot ./cmd/zfs-auto-snapshot
ln -f zfs-auto-snapshot zfs-cleanup-snapshots
ln -f zfs-auto-snapshot zfs-snapshot-mysql
```

You can then install the binary and create the links in your system path:

```sh
sudo install zfs-auto-snapshot /usr/local/sbin/
sudo ln -f /usr/local/sbin/zfs-auto-snapshot /usr/local/sbin/zfs-cleanup-snapshots
sudo ln -f /usr/local/sbin/zfs-auto-snapshot /usr/local/sbin/zfs-snapshot-mysql
```

The links must be on the same filesystem. Recreate both links when upgrading so all three names point to the newly installed inode.

---

## Usage

### `zfs-auto-snapshot`

```
Usage: /usr/local/sbin/zfs-auto-snapshot [-dknpuv] [-P pool] [-s prefix] <INTERVAL> <KEEP>
  -d              Show debug output.
  -k              Keep zero-sized snapshots.
  -n              Do a dry-run. Nothing is committed. Only show what would be done.
  -p              Create snapshots in parallel.
  -P pool         Act only on the specified pool.
  -s prefix       Set the generated snapshot prefix.
  -u              Use UTC for snapshots.
  -v              Show what is being done.
  INTERVAL        The interval to snapshot (e.g., hourly, daily).
  KEEP            Total snapshots to retain; 0 only cleans up.
```

`KEEP` is the total number of matching snapshots left after the command
completes, including the newly created snapshot. With `KEEP=0`, no snapshot is
created and all matching snapshots for the interval are removed from included
datasets.

Database coordination currently derives from the general
`com.sun:auto-snapshot` property. An interval-specific `mysql` or `postgresql`
value can make a dataset eligible without selecting the corresponding database
workflow. See [Known Limitations](IMPLEMENTATIONS.md#known-limitations).

### `zfs-cleanup-snapshots`

```
Usage: /usr/local/sbin/zfs-cleanup-snapshots [-dnpv] [-P pool]
    -d              Show debug output.
    -n              Do a dry-run. Nothing is committed. Only show what would be done.
    -p              Destroy snapshots in parallel.
    -P pool         Act only on the specified pool.
    -v              Show what is being done.
```

> **Warning:** `zfs-cleanup-snapshots` recognizes only the default
> `zfs-auto-snap_` prefix as automatic. Snapshots made with a custom `-s`
> prefix are treated as manual snapshots and can enter zero-size cleanup.

### `zfs-snapshot-mysql`

```
Usage: /usr/local/sbin/zfs-snapshot-mysql [-dnv] DATASET
    -d              Show debug output.
    -n              Do a dry-run. Nothing is committed. Only show what would be done.
    -v              Show what is being done.
```

## Future Work

- Evaluate Lua-based ZFS Channel Programs (ZCP) for atomic ZFS-side operations.

---

## Credits

Directly derived from the Ruby `zfstools` project originally written by Bryan
Drewery. The automatic snapshot property and naming conventions originated in
the OpenSolaris projects referenced above.
