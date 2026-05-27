#!/bin/sh
set -eu

if [ -d "${ASR_MYSQL_DATA_DIR}/mysql" ]; then
  exit 0
fi

mysqld --initialize-insecure --user=mysql --datadir="${ASR_MYSQL_DATA_DIR}"

/usr/sbin/mysqld --user=mysql --datadir="${ASR_MYSQL_DATA_DIR}" --socket=/run/mysqld/mysqld.sock --pid-file=/run/mysqld/mysqld.pid --skip-networking &
MYSQL_BOOTSTRAP_PID=$!

cleanup() {
  if kill -0 "${MYSQL_BOOTSTRAP_PID}" 2>/dev/null; then
    kill "${MYSQL_BOOTSTRAP_PID}" 2>/dev/null || true
    wait "${MYSQL_BOOTSTRAP_PID}" 2>/dev/null || true
  fi
}

trap cleanup INT TERM EXIT

for _ in $(seq 1 60); do
  if mysqladmin --socket=/run/mysqld/mysqld.sock ping --silent >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

mysql --socket=/run/mysqld/mysqld.sock <<SQL
ALTER USER 'root'@'localhost' IDENTIFIED BY '${ASR_MYSQL_ROOT_PASSWORD}';
CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' IDENTIFIED BY '${ASR_MYSQL_ROOT_PASSWORD}';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION;
CREATE DATABASE IF NOT EXISTS \
	\`${ASR_MYSQL_DATABASE}\` \
	CHARACTER SET utf8mb4 \
	COLLATE utf8mb4_unicode_ci;
FLUSH PRIVILEGES;
SQL

mysqladmin --socket=/run/mysqld/mysqld.sock -uroot -p"${ASR_MYSQL_ROOT_PASSWORD}" shutdown
wait "${MYSQL_BOOTSTRAP_PID}"
trap - INT TERM EXIT