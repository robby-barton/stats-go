#!/bin/sh
set -e

: "${PG_USER:?PG_USER is required}"
: "${PG_PASSWORD:?PG_PASSWORD is required}"
: "${PG_HOST:?PG_HOST is required}"
: "${PG_PORT:?PG_PORT is required}"
: "${PG_DBNAME:?PG_DBNAME is required}"
: "${CF_PAGES_PROJECT:?CF_PAGES_PROJECT is required}"

REF="${DEPLOY_REF:-master}"

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

# Shallow clone with retries: the network (or GitHub) can hiccup, and a
# transient failure should not abort the whole deploy.
clone_ok=0
attempt=1
max_attempts=3
while [ "$attempt" -le "$max_attempts" ]; do
    if git clone --depth 1 --single-branch --branch "$REF" \
        https://github.com/robby-barton/stats-web.git "$WORK_DIR"; then
        clone_ok=1
        break
    fi
    echo "deploy: git clone attempt $attempt/$max_attempts failed" >&2
    rm -rf "$WORK_DIR"
    if [ "$attempt" -lt "$max_attempts" ]; then
        sleep 5
    fi
    attempt=$((attempt + 1))
done
if [ "$clone_ok" -ne 1 ]; then
    echo "deploy: git clone of stats-web (ref $REF) failed after $max_attempts attempts" >&2
    exit 1
fi

cd "$WORK_DIR"
echo "deploy: stats-web commit $(git rev-parse --short=8 HEAD)"

export DATABASE_URL="postgresql://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DBNAME}"

yarn install --frozen-lockfile
yarn build
# wrangler is a pinned devDependency, so this resolves locally without npx.
# --branch routes master to the CF Pages production branch; other refs deploy
# as previews.
yarn wrangler pages deploy _site/ --project-name "${CF_PAGES_PROJECT}" --branch "$REF"
