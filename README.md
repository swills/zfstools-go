# zfstools-go

**zfstools-go** is a faithful reimplementation of the original [zfstools Ruby project](https://github.com/bdrewery/zfstools), rewritten in Go with equivalent behavior and improved error handling.

This toolkit provides automated ZFS snapshot management using three tools:

- `zfs-auto-snapshot`
- `zfs-cleanup-snapshots`
- `zfs-snapshot-mysql`

All command-line options, behaviors, and output formats exactly match the original Ruby tools.

---

## Features

- Fully compatible with FreeBSD's `zfs` and `zpool` CLI utilities
- Automated recursive and interval-based snapshot creation
- Dry-run and verbose modes for safe operation
- Intelligent pruning of expired or zero-sized snapshots
- Optional MySQL-aware snapshot locking

---

## Installation

Build with Go 1.23 or later:

```sh
go build -o zfs-auto-snapshot ./cmd/zfs-auto-snapshot
go build -o zfs-cleanup-snapshots ./cmd/zfs-cleanup-snapshots
go build -o zfs-snapshot-mysql ./cmd/zfs-snapshot-mysql
```

You can then install them in your system path:

```sh
sudo install zfs-auto-snapshot /usr/local/sbin/
sudo install zfs-cleanup-snapshots /usr/local/sbin/
sudo install zfs-snapshot-mysql /usr/local/sbin/
```

---

## Usage

### `zfs-auto-snapshot`

```
Usage: /usr/local/sbin/zfs-auto-snapshot [-dknpuv] <INTERVAL> <KEEP>
  -d              Show debug output.
  -k              Keep zero-sized snapshots.
  -n              Do a dry-run. Nothing is committed. Only show what would be done.
  -p              Create snapshots in parallel.
  -P pool         Act only on the specified pool.
  -u              Use UTC for snapshots.
  -v              Show what is being done.
  INTERVAL        The interval to snapshot (e.g., hourly, daily).
  KEEP            How many snapshots to retain for this interval.
```

### `zfs-cleanup-snapshots`

```
Usage: /usr/local/sbin/zfs-cleanup-snapshots [-dnv]
    -d              Show debug output.
    -n              Do a dry-run. Nothing is committed. Only show what would be done.
    -p              Destroy snapshots in parallel.
    -P pool         Act only on the specified pool.
    -v              Show what is being done.
```

### `zfs-snapshot-mysql`

```
Usage: /usr/local/sbin/zfs-snapshot-mysql [-dnv] DATASET
    -d              Show debug output.
    -n              Do a dry-run. Nothing is committed. Only show what would be done.
    -v              Show what is being done.
```

---

## Credits

Originally written in Ruby by Bryan Drewery  
