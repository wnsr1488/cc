#!/bin/sh
# Output: "<count> <ip>" per line for non-whitelisted remote peers with established TCP connections.
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
