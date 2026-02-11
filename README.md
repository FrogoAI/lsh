🚀 High-Performance MinHash LSH for Go

A production-grade Locality Sensitive Hashing (LSH) implementation engineered for high-throughput deduplication, fraud detection, and similarity search. This library focuses on extreme CPU efficiency and zero-allocation hot paths.
✨ Key Features

    ⚡ Odd-Coefficient Optimization: Replaces expensive modulo arithmetic with implicit 264 overflow. By enforcing odd coefficients, we guarantee a mathematically perfect permutation with zero CPU overhead.

    ♻️ Zero-Allocation Design: Utilizes sync.Pool for signature buffers and optimized slicing to eliminate GC pressure during heavy ingestion.

    🛡️ Hot-Bucket Protection: Includes a "Peek & Stub" strategy for Aerospike/Database layers to prevent OOM crashes when encountering "hot" tokens (millions of matches).

    🧩 Scalable Architecture: Decoupled storage interfaces supporting In-Memory and Aerospike (CDT-based) backends out of the box.

    🧪 Mathematically Sound: Implements 3-char shingling and MinHash signatures with configurable P=1−(1−sr)b probability curves.

📦 Installation
Bash

go get github.com/FrogoAI/lsh

🚀 Quick Start
Go

package main

import (
"context"
"github.com/FrogoAI/lsh"
"github.com/FrogoAI/lsh/repositories/memory"
)

func main() {
// 1. Setup Config
cfg := &lsh.Config{
Bands:            20,
Rows:             5,
ShingleSize:      3,
JaccardThreshold: 0.6,
Seed:             13374269,
}

    // 2. Initialize Service
    repo := memory.New()
    service := lsh.NewSimilarityService(repo, cfg)

    // 3. Upsert a record
    // Returns "" if it's a new unique record, or the existing ID if it's a duplicate.
    id, _ := service.Upsert(context.Background(), "users", "John Doe")
}

📐 Tuning the LSH Curve

The probability of two items sharing a bucket depends on their Jaccard similarity s, the number of bands b, and rows per band r.

    Higher Recall (Catch more): Increase Bands.

    Higher Precision (Fewer false positives): Increase Rows.

🔬 Performance Optimizations
The "Odd Coefficient" Trick

Standard MinHash often uses (ax+b)(modP). We utilize the CPU's natural behavior with 264 overflow. Because we ensure a is always odd, it remains coprime to 264, satisfying the requirement for a linear congruential generator to produce a full-period permutation.
Aerospike CDT Implementation

The Aerospike repository uses Map CDTs (Collection Data Types). Instead of fetching a whole list of IDs, we use MapSizeOp to check the density of a bucket server-side. If a bucket is too "hot," we skip it without ever transferring the data over the wire.