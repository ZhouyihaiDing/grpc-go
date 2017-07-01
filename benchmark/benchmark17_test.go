// +build go1.7

/*
 *
 * Copyright 2017 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package benchmark

import (
	"fmt"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc/benchmark/stats"
)

func BenchmarkClient(b *testing.B) {
	maxConcurrentCalls := 1
	reqSizeBytes := 1
	reqspSizeBytes := 1
	kbps := 0 // if non-positive, infinite
	MTU := 0    // if non-positive, infinite
	// When set the latency to 0 (no delay), the result is slower than the real result with no delay
	// because latency simulation section has extra operations
	latency := time.Duration(0 * time.Millisecond)

	b.Run(fmt.Sprintf("Unary-%s-kbps_%#v-MTU_%#v-maxConcurrentCalls_"+
			"%#v-reqSize_%#vB-respSize_%#vB-latency_%s",
		"tracing", kbps, MTU, maxConcurrentCalls, reqSizeBytes, reqspSizeBytes, "1s"), func(b *testing.B) {
									runUnary(func() {
										b.StartTimer()
									}, func() {
										b.StopTimer()
									}, func(ch chan int) {
										for i := 0; i < b.N; i++ {
											ch <- 1
										}
									},
										maxConcurrentCalls, reqSizeBytes, reqspSizeBytes, kbps, MTU, latency)
								})


}

func TestMain(m *testing.M) {
	os.Exit(stats.RunTestMain(m))
}
