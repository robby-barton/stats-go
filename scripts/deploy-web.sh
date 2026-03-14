#!/bin/sh
set -e

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

git clone https://github.com/robby-barton/stats-web.git "$WORK_DIR"
cd "$WORK_DIR"

export DATABASE_URL="postgresql://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DBNAME}"

yarn install --frozen-lockfile
yarn build
npx wrangler pages deploy _site/ --project-name "${CF_PAGES_PROJECT}"
