#!/bin/sh
# Embedded monitor collection script (run over SSH).
read _ u1 n1 s1 i1 iw1 ir1 si1 st1 _ _ _ _ _ _ < /proc/stat
sleep 1
read _ u2 n2 s2 i2 iw2 ir2 si2 st2 _ _ _ _ _ _ < /proc/stat
idle1=$((i1 + iw1))
idle2=$((i2 + iw2))
total1=$((u1 + n1 + s1 + idle1 + ir1 + si1 + st1))
total2=$((u2 + n2 + s2 + idle2 + ir2 + si2 + st2))
dt=$((total2 - total1))
if [ "$dt" -gt 0 ]; then
  cpu=$(awk "BEGIN {printf \"%.2f\", ($dt - ($idle2 - $idle1)) * 100 / $dt}")
else
  cpu=0
fi

mem=$(awk '/MemTotal/{t=$2}/MemAvailable/{a=$2}END{if(t>0) printf "%.2f", (t-a)*100/t; else print 0}' /proc/meminfo)
disk=$(df -P / 2>/dev/null | awk 'NR==2 {gsub(/%/,"",$5); print $5+0}')
read load1 load5 load15 _ < /proc/loadavg
est=$(ss -H -ant state established 2>/dev/null | wc -l)
tw=$(ss -H -ant state time-wait 2>/dev/null | wc -l)
read net_in net_out <<EOF
$(awk 'NR>2 && $1 !~ /^(lo|docker|veth|br-|virbr)/ {gsub(/:/,"",$1); in+=$2; out+=$10} END {print in+0, out+0}' /proc/net/dev)
EOF
net_in=${net_in:-0}
net_out=${net_out:-0}
blocked=0
for set in cc_blacklist cc_temp_block cc_rate_block; do
  if ipset list "$set" >/tmp/cc-panel-mon-ipset 2>/dev/null; then
    count=$(awk '/Number of entries:/ {print $4}' /tmp/cc-panel-mon-ipset)
    blocked=$((blocked + ${count:-0}))
  fi
done
rm -f /tmp/cc-panel-mon-ipset
drops=$(iptables -L INPUT -v -n -x 2>/dev/null | awk '/DROP/ && /cc_/ {sum+=$1} END {print sum+0}')
echo "$cpu $mem $disk $load1 $load5 $load15 $est $tw $net_in $net_out $blocked $drops"
