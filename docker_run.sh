docker run -d \
    --net=host \
    -v /data/haproxy:/etc/haproxy \
    -v /data/bamboo:/config \
    -e MARATHON_ENDPOINT=http://192.168.1.127:5098 \
    -e BAMBOO_ENDPOINT=http://192.168.1.127:8000 \
    -e BAMBOO_ZK_HOST=192.168.1.127:5092 \
    -e MARATHON_USE_EVENT_STREAM=true \
    -e BAMBOO_ZK_PATH=/bb_gateway \
    -e APPLICATION_ID=test-2048-001 \
    -e BIND=":8000" \
    -e CONFIG_PATH="config/production.json" \
    --name=bamboo2 \
catalog.shurenyun.com/library/omega-bamboo:omega.v3.0.1 

