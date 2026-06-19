#!/bin/sh
set -eu

mkdir -p /var/lib/tor/sprag
chown -R debian-tor:debian-tor /var/lib/tor
chmod 700 /var/lib/tor /var/lib/tor/sprag

exec tor -f /etc/tor/torrc
