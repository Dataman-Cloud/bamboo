docker run -d \
	-e BIND=0.0.0.0:5004 \
	-e CONFIG_PATH=/config/production.json \
	-v /data/haproxy:/etc/haproxy \
	-v /data/run/haproxy:/run/haproxy \
	--name haproxy1 --net host \
catalog.shurenyun.com/library/omega-haproxyctl:omega.v2.4.2

