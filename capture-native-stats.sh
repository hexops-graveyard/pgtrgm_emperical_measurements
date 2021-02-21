# Usage: OUT=native_stats_logs/foobar ./capture-native-stats.sh
watch -n 1 "(date '+TIME:%H:%M:%S'; top -n 1 -l 1) | tee -a $OUT"
