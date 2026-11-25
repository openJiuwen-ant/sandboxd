#!/usr/bin/env bash
# Run on a seed-enabled sandboxd node with static BusyBox and mkfs.erofs.
set -euo pipefail

: "${RUNSC_BINARY:?}" "${SEED_HELPER:?}" "${RUNSC_ROOT:?}" "${SOCKET:?}"
platform=${RUNSC_PLATFORM:-kvm}
sbox=${SBOX_BINARY:-sbox}
fixture=$(mktemp -d)
seed=seed-${fixture##*.}
a=sbox-${seed}-a
b=sbox-${seed}-b
client=("$sbox" --address "$SOCKET" --timeout 30s)
helper=("$SEED_HELPER" "--runsc=$RUNSC_BINARY" "--root=$RUNSC_ROOT" "--platform=$platform" --cpu-num=1 --network=none --directfs=false --ignore-cgroups)
runsc=("$RUNSC_BINARY" "--root=$RUNSC_ROOT")
cleanup() {
    "${client[@]}" delete "$a" "$b" >/dev/null 2>&1 || true
    "${helper[@]}" delete "$seed" >/dev/null 2>&1 || true
    if [[ ${KEEP_SEED_FIXTURE:-0} = 1 ]]; then echo "fixture=$fixture"; else rm -rf "$fixture"; fi
}
trap cleanup EXIT
mkdir -p "$fixture/image"/{tmp,proc,dev,sys} "$fixture/rootfs"
cp "${BUSYBOX:-/bin/busybox}" "$fixture/image/busybox"
cat >"$fixture/image/warm.sh" <<'GUEST'
set -eu
read -r warm </proc/sys/kernel/random/uuid
exec 3<>/proc/gvisor/checkpoint
printf 1 >&3
read -r action <&3
test "$action" = restore
exec 3>&-
echo "$warm" >/tmp/inherited
exec /busybox sleep 3600
GUEST
mkfs.erofs -E noinline_data "$fixture/root.erofs" "$fixture/image" >/dev/null
jq -n --arg img "$fixture/root.erofs" '{
    ociVersion:"1.0.2",root:{path:"rootfs",readonly:true},
    process:{args:["/busybox","sh","/warm.sh"],env:["PATH=/"],cwd:"/",user:{uid:0,gid:0}},
    mounts:[{destination:"/tmp",type:"tmpfs",source:"tmpfs",options:["size=16m"]}],
    linux:{namespaces:[{type:"pid"},{type:"mount"},{type:"ipc"},{type:"uts"},{type:"network"}]},
    annotations:{"dev.gvisor.spec.rootfs.type":"erofs","dev.gvisor.spec.rootfs.source":$img,"dev.gvisor.spec.rootfs.overlay":""}
}' >"$fixture/config.json"
"${helper[@]}" "--bundle=$fixture" "--stdout=$fixture/seed.out" "--stderr=$fixture/seed.err" create "$seed"
for id in "$a" "$b"; do
    "${client[@]}" start --template-id "$seed" --sandbox-id "$id" --env "CHILD=$id" --memory-mb 512 --quiet
done
if "${client[@]}" start --quiet --template-id "$seed" --mount "$fixture:/work" >"$fixture/rejected" 2>&1; then
    echo "start accepted a mount override" >&2
    exit 1
fi
grep -q 'template_id inherits' "$fixture/rejected"
guest() { "${runsc[@]}" exec "$1" /busybox "${@:2}"; }
first=$(guest "$a" cat /tmp/inherited)
second=$(guest "$b" cat /tmp/inherited)
[[ -n $first && $first = "$second" ]]
guest "$a" touch /tmp/only-a
guest "$b" test ! -e /tmp/only-a
for id in "$a" "$b"; do
    state=$("${runsc[@]}" state "$id")
    config=$(jq -r .bundle <<<"$state")/config.json
    cg=$(jq -r '.linux.cgroupsPath // ""' "$config")
    if [[ -n $cg ]]; then
        [[ $(jq -r .linux.resources.memory.limit "$config") = 536870912 ]]
        grep -F -- "$cg" "/proc/$(jq -r .pid <<<"$state")/cgroup"
    fi
    guest "$id" cat /proc/gvisor/spec_environ | tr '\0' '\n' | grep -Fx "CHILD=$id"
done
guest_ip() { guest "$1" ip -o -4 addr | awk '!/ lo / {print $4}'; }
ip_a=$(guest_ip "$a")
ip_b=$(guest_ip "$b")
[[ -n $ip_a && -n $ip_b && $ip_a != "$ip_b" ]]
guest "$a" httpd -p 8099 -h /tmp
[[ $(guest "$b" wget -q -T 5 -O - "http://${ip_a%/*}:8099/inherited") = "$first" ]]
"${client[@]}" delete "$a"
if "${runsc[@]}" state "$a" >/dev/null 2>&1; then exit 1; fi
[[ $(guest "$b" cat /tmp/inherited) = "$first" ]]
"${client[@]}" delete "$b"
if "${runsc[@]}" state "$b" >/dev/null 2>&1; then exit 1; fi
echo "PASS platform=$platform inherited=$first child_a=$ip_a child_b=$ip_b tcp=ok child_delete=ok"
