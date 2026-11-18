#!/bin/bash
# Daily logical backup of the Kinjo database (run by kinjo-backup.timer).
# Restore (decrypt a .age file first, see below): pg_restore --clean --if-exists --no-owner -d "$DATABASE_URL" <file>.dump
# Prove backups work by restoring one into a scratch DB monthly — an untested
# backup is a hope, not a backup.
set -euo pipefail
DIR=/var/backups/kinjo
KEEP_DAYS=14
# pool_max_conns is a pgx setting; libpq tools reject unknown URL params.
URL=$(sed -E 's/pool_max_conns=[0-9]+&?//; s/[?&]$//' <<<"$DATABASE_URL")

mkdir -p "$DIR"
f="$DIR/kinjo-$(date +%F-%H%M).dump"
# Dumps hold every email, photo and credential: with BACKUP_AGE_RECIPIENT set (an age
# public key, age1...), each is encrypted before it touches the disk. Only the public
# key lives on the server, so someone who takes the server can't read old backups;
# the private key stays offline (restore: age -d -i key.txt f.dump.age > f.dump).
umask 077
if [ -n "${BACKUP_AGE_RECIPIENT:-}" ]; then
  f="$f.age"
  pg_dump --format=custom --no-owner "$URL" | age -r "$BACKUP_AGE_RECIPIENT" >"$f.tmp" && mv "$f.tmp" "$f"
else
  pg_dump --format=custom --no-owner "$URL" >"$f.tmp" && mv "$f.tmp" "$f"
fi
find "$DIR" -name 'kinjo-*.dump*' -mtime +"$KEEP_DAYS" -delete

# A backup on the same disk doesn't survive the disk. Once we pick a destination,
# set BACKUP_REMOTE in the .env (an rclone remote, e.g. utho-s3:kinjo-backups).
# Never off the box unencrypted.
if [ -n "${BACKUP_REMOTE:-}" ]; then
  if [ -z "${BACKUP_AGE_RECIPIENT:-}" ]; then echo "BACKUP_REMOTE needs BACKUP_AGE_RECIPIENT: not uploading a plaintext dump" >&2; exit 1; fi
  rclone copy "$f" "$BACKUP_REMOTE"
fi
echo "backup ok: $f ($(du -h "$f" | cut -f1))"
