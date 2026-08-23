# ZFS-on-Linux Shell Rewrite Evaluation

This document compares `zfstools-go` with the independent shell rewrite at
[zfsonlinux/zfs-auto-snapshot](https://github.com/zfsonlinux/zfs-auto-snapshot).
That repository was initially mistaken for the historical original. The real
Sun implementation and the broader lineage are documented in
[`IMPLEMENTATIONS.md`](IMPLEMENTATIONS.md).

The shell rewrite remains useful as a source of operational lessons, but it is
not authoritative for this project's command line or policy.

## Provenance and Compatibility

The shell project was started by Darik Horn as a Linux implementation. Its main
sources are `src/zfs-auto-snapshot.sh` and `src/zfs-auto-snapshot.8`.

`zfstools-go` is instead a Go reimplementation of Ruby `zfstools`. The two
commands share a common name but have incompatible interfaces:

| Area | Linux shell rewrite | zfstools-go |
| --- | --- | --- |
| Invocation | Options plus explicit datasets or `//` | Required `INTERVAL KEEP` |
| Dataset default | Opt-out; `--default-exclude` enables opt-in | Always Ruby-derived opt-in |
| Recursion | Explicit `--recursive` | Automatic safe recursive partitioning |
| Retention | Optional positive `--keep` | Required non-negative `KEEP` |
| Cleanup-only | `--destroy-only` | `KEEP=0` |
| Naming | Configurable prefix, separator, label; UTC | Prefix plus interval; local or trailing-`U` UTC |
| Database handling | Arbitrary pre/post hooks | Ruby-derived database coordination from the general property |
| Parallel creation | No equivalent | Optional individual parallel creation |

Short options also conflict. For example, shell `-k`, `-p`, and `-s` mean
keep, prefix, and skip-scrub, while the Go command uses them for keep-zero-size,
parallel operation, and snapshot prefix. Existing shell cron entries are not
safe inputs to the Go binary.

## Safety Lessons Already Adopted

The comparison identified real weaknesses in the earlier Go port even though
the shell was not the original implementation. Those findings have been
resolved:

- Dataset and snapshot discovery now fail closed instead of returning partial
  or empty successful results.
- Unknown snapshot size is distinct from a confirmed zero.
- Generated-snapshot retention matches the exact snapshot component, prefix,
  interval, and timestamp grammar.
- Retention uses numeric ZFS creation time with a deterministic name
  tie-breaker.
- Creation returns confirmed targets, and retention runs only for those
  targets. Failed pooled or recursive commands authorize no cleanup for their
  indeterminate targets.
- Serial cleanup stops on failure; parallel cleanup joins launched failures.
- Context cancellation remains visible before, during, and after cleanup.
- Dry run always prints every proposed mutation, including database commands,
  while still performing discovery and size checks.
- Parallel dry-run output is synchronized.

These are `zfstools-go` safety guarantees, not shell-compatibility behavior.

## Deliberately Different Policies

### Dataset eligibility

The shell normally includes datasets unless an effective property contains
`false`. `--default-exclude` reverses that default. It does not require
filesystems to be mounted.

The Go command keeps Ruby's opt-in policy: a mounted filesystem or volume must
resolve to the exact value `true`, `mysql`, or `postgresql`, with the
interval-specific property considered before the general property.

### Targets and recursion

The shell accepts positional datasets or `//`, and recursion is requested by an
option. The Go command discovers eligible datasets, optionally within one pool,
and automatically chooses individual or recursive commands without crossing an
excluded descendant.

### Names

The shell uses:

```text
<prefix><separator><label>-YYYY-MM-DD-HHMM
```

It uses UTC and can attach `com.sun:auto-snapshot-desc`. The Go command retains
the Ruby/Time Slider form:

```text
<prefix>_<interval>-YYYY-MM-DD-HHhMM
<prefix>_<interval>-YYYY-MM-DD-HHhMMU
```

Local time is the default and `U` marks UTC.

### Retention and cleanup-only operation

The shell's `--keep` is optional and positive. It reserves one retained slot
for a prospective new snapshot. In destroy-only or minimum-size-skipped paths,
that assumption creates an off-by-one result because no new snapshot exists.

The Go command defines `KEEP` as the final total. `KEEP=0` is intentionally
cleanup-only. Dry run accounts for a proposed snapshot that is absent from ZFS
discovery so its plan matches a real run.

### Zero-size cleanup

The shell does not prune snapshots according to snapshot `used`; its
`--min-size` option examines dataset `written` before creation. The Go command
retains Ruby's zero-size policies while protecting the newest snapshot and
aborting all planned deletion when a size is unknown.

## Features Not Adopted

The shell offers useful features that could be considered independently:

- Explicit positional dataset targets.
- Pool readiness and optional scrub avoidance.
- Event metadata.
- Pre/post snapshot hooks.
- A minimum-written-data threshold.
- Quiet/syslog output and operation summaries.
- Configurable labels and separators for migration.

They are not implicit compatibility requirements. Any implementation should
define validation, cancellation, recursive and pooled semantics, and failure
gating rather than copying shell command construction.

## Behavior Not to Copy

- Reconstructed shell pipelines, `eval`, and glob/regular-expression matching
  of dataset names.
- The name-ordered `--fast` approximation.
- Destroy-only retention's missing-snapshot off-by-one behavior.
- A pre-hook failure state that can affect later unrelated targets.
- Mutation failures that only increment warning counters while final command
  status remains successful.
- Loose validation of minimum-size values and command output.

## Conclusion

The Linux shell rewrite is neither the historical original nor the direct
ancestor of this project. It is a separate implementation with useful features
and useful cautionary examples. `zfstools-go` does not target command-line or
policy compatibility with it.
