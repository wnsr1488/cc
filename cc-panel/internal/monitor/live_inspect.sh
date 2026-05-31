#!/bin/sh
# Live blocked-ip and per-source connection snapshot (run over SSH).
echo "__CC_ESTAB__"
ss -H -ant state established 2>/dev/null | wc -l
echo "__CC_BLOCKED__"
for set in cc_blacklist cc_temp_block cc_rate_block; do
  ipset list "$set" 2>/dev/null | awk -v s="$set" '
    /^Members:/ { in_members=1; next }
    in_members && NF==0 { in_members=0 }
    in_members && $1 ~ /^[0-9a-fA-F:\.*]+$/ {
      ip = $1
      if (ip ~ /^::ffff:/) ip = substr(ip, 8)
      t=0
      if ($2=="timeout") t=$3
      print s "\t" ip "\t" t
    }
  '
done
echo "__CC_CONN__"
ss -H -nt state established 2>/dev/null | awk '{
  peer = $4
  sub(/:[0-9]+$/, "", peer)
  gsub(/^\[|\]$/, "", peer)
  if (peer ~ /^::ffff:/) peer = substr(peer, 8)
  if (peer == "" || peer ~ /^127\./ || peer == "::1") next
  counts[peer]++
}
END {
  for (ip in counts) print counts[ip], ip
}' | sort -rn | while read -r count ip; do
  [ -z "$ip" ] && continue
  case "$ip" in
    ::ffff:*) ip="${ip#::ffff:}" ;;
  esac
  ipset test cc_whitelist "$ip" 2>/dev/null && continue
  ipset test cc_geo_whitelist "$ip" 2>/dev/null && continue
  echo "$count $ip"
done
