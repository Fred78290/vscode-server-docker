#!/bin/sh

cp /tmp/redis/redis.conf /etc/redis/redis.conf

echo "finding master..."
MASTER_FDQN=`hostname  -f | sed -e 's/redis-[0-9]\./redis-0./'`

if [ "$(redis-cli -h sentinel -p 5000 ping)" != "PONG" ]; then
	echo "master not found, defaulting to redis-0"

	if [ "$(hostname)" == "redis-0" ]; then
		echo "this is redis-0, not updating config..."
	else
		echo "updating redis.conf..."
		echo "slaveof $MASTER_FDQN 6379" >> /etc/redis/redis.conf
	fi
else
	echo "sentinel found, finding master"
	MASTER="$(redis-cli -h sentinel -p 5000 sentinel get-master-addr-by-name mymaster | grep -E '(^redis-\d{1,})|([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})')"
	echo "master found : $MASTER, updating redis.conf"
	echo "slaveof $MASTER 6379" >> /etc/redis/redis.conf
fi
