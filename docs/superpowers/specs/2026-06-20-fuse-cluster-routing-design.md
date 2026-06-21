# FUSE Cluster-Aware Chunk Routing Design

**Date:** 2026-06-20  
**Status:** approved  

## Goal

挂载任意集群节点即可访问全部数据；多 seed 高可用；chunk 按 registry 副本位置直连数据节点。

## Data Flow

```
FUSE Read/Write
  → RemoteMeta (any seed node) — metadata
  → ListNodes — discover all cluster nodes
  → GET /v1/chunks/registry/get — replica URLs for chunk
  → HTTP GET/PUT /memoryfs/chunks/{id} — direct to data node
```

## Changes

- `RefreshNodes`: merge seeds + `/v1/cluster/nodes`
- `nodesForChunk`: registry replicas → hash fallback
- API: `POST /v1/chunks/registry/get`
- Meta reads try all seeds; writes redirect to leader

## HA

- `-nodes url1,url2,...` — multiple seeds
- Chunk read/write tries each replica node until success
- Heartbeat refreshes node list every 60s (`-v`)
