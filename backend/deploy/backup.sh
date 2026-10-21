#!/bin/bash
# Daily logical backup of the Kinjo database (run by kinjo-backup.timer).
# Restore: pg_restore --clean --if-exists --no-owner -d "$DATABASE_URL" <file>.dump
# Prove backups work by restoring one into a scratch DB monthly — an untested
# backup is a hope, not a backup.
set -euo pipefail
DIR=/var/backups/kinjo
KEEP_DAYS=14
# pool_max_conns is a pgx setting; libpq tools reject unknown URL params.
URL=$(sed -E 's/pool_max_conns=[0-9]+&?//; s/[?&]$//' <<<"$DATABASE_URL")

mkdir -p "$DIR"
f="$DIR/kinjo-$(date +%F-%H%M).dump"
pg_dump --format=custom --no-owner "$URL" >"$f.tmp" && mv "$f.tmp" "$f"
find "$DIR" -name 'kinjo-*.dump' -mtime +"$KEEP_DAYS" -delete

# A backup on the same disk doesn't survive the disk. Once we pick a destination,
# set BACKUP_REMOTE in the .env (an rclone remote, e.g. utho-s3:kinjo-backups).
if [ -n "${BACKUP_REMOTE:-}" ]; then rclone copy "$f" "$BACKUP_REMOTE"; fi
echo "backup ok: $f ($(du -h "$f" | cut -f1))"
