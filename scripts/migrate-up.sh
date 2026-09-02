#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
MIGRATE_BIN=${MIGRATE_BIN:-migrate}
DATABASE_DRIVER=${DATABASE_DRIVER:-postgres}
DATABASE_DSN=${DATABASE_DSN:-}

if [ -z "$DATABASE_DSN" ]; then
	printf '%s\n' 'DATABASE_DSN is required' >&2
	exit 1
fi

case "$DATABASE_DRIVER" in
	postgres|postgresql|pgx)
		DATABASE_URL=$DATABASE_DSN
		;;
	sqlite|sqlite3)
		case "$DATABASE_DSN" in
			sqlite3://*) DATABASE_URL=$DATABASE_DSN ;;
			*) DATABASE_URL="sqlite3://$DATABASE_DSN" ;;
		esac
		;;
	*)
		printf 'unsupported database driver: %s\n' "$DATABASE_DRIVER" >&2
		exit 1
		;;
esac

"$MIGRATE_BIN" -source "file://$ROOT_DIR/migrations" -database "$DATABASE_URL" up
