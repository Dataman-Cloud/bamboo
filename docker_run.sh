docker run -d \
    --net=host \
    -v /data/haproxy:/etc/haproxy \
    -e MARATHON_ENDPOINT=http://192.168.1.155:8080 \
    -e BAMBOO_ENDPOINT=http://192.168.1.127:8000 \
    -e BAMBOO_ZK_HOST=192.168.1.155:2181 \
    -e MARATHON_USE_EVENT_STREAM=true \
    -e BAMBOO_ZK_PATH=/bb_gateway \
    -e APPLICATION_ID=test-2048-001 \
    -e BIND=":8000" \
    -e CONFIG_PATH="config/production.json" \
    --name=bamboo \
catalog.shurenyun.com/library/omega-bamboo:omega.v3.0 

