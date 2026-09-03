#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

if [ -f "$ROOT_DIR/.env" ]; then
	set -a
	. "$ROOT_DIR/.env"
	set +a
fi

if [ -z "${MIGRATE_BIN+x}" ] && command -v go >/dev/null 2>&1; then
	GO_MIGRATE_BIN="$(go env GOPATH)/bin/migrate"
	if [ -x "$GO_MIGRATE_BIN" ]; then
		MIGRATE_BIN=$GO_MIGRATE_BIN
	fi
fi
MIGRATE_BIN=${MIGRATE_BIN:-migrate}
APP_ENV=${APP_ENV:-development}
DATABASE_DRIVER=${DATABASE_DRIVER:-postgres}
DATABASE_DSN=${DATABASE_DSN:-}
MIGRATE_STEPS=${1:-1}

case "$APP_ENV" in
	development|test) ;;
	*)
		printf '%s\n' 'migrate-down is allowed only in development or test' >&2
		exit 1
		;;
esac

case "$MIGRATE_STEPS" in
	''|0|*[!0-9]*)
		printf '%s\n' 'migration steps must be a positive integer' >&2
		exit 1
		;;
esac

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
			:memory:|file:*|sqlite3://*) DATABASE_URL=${DATABASE_DSN#sqlite3://} ;;
			*)
				mkdir -p "$(dirname "$DATABASE_DSN")"
				DATABASE_URL="sqlite3://$DATABASE_DSN"
				;;
		esac
		;;
	*)
		printf 'unsupported database driver: %s\n' "$DATABASE_DRIVER" >&2
		exit 1
		;;
esac

"$MIGRATE_BIN" -source "file://$ROOT_DIR/migrations" -database "$DATABASE_URL" down "$MIGRATE_STEPS"
