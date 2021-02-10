# Usage: OUT=docker_stats_logs/foobar ./capture-docker-stats.sh
watch -n 1 "(date '+TIME:%H:%M:%S'; docker stats --no-stream --no-trunc --format '{{json .}}') | tee -a $OUT"
