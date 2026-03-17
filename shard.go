package lsh

import "hash/fnv"

const GroupLockShards = 64

// GroupShard maps a group string to a shard index for lock striping.
func GroupShard(group string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(group)) //nolint:errcheck

	return h.Sum32() % GroupLockShards
}
