// +build go1.7

package benchmark

import (
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/benchmark/stats"
)

func BenchmarkClient(b *testing.B) {
	maxConcurrentCalls := []int{1, 8, 64, 512}
	reqSizeBytes := []int{1, 1024}
	reqspSizeBytes := []int{1, 1024}
	s := stats.NewStats(38)
	isMatched := false
	for _, enableTracing := range []bool{true, false} {
		grpc.EnableTracing = enableTracing
		tracing := "Tracing"
		if !enableTracing {
			tracing = "noTrace"
		}
		for _, maxC := range maxConcurrentCalls {
			for _, reqS := range reqSizeBytes {
				for _, respS := range reqspSizeBytes {
					b.Run(fmt.Sprintf("Unary-%s-maxConcurrentCalls_"+
							"%#v-reqSize_%#vB-respSize_%#vB", tracing, maxC, reqS, respS), func(b *testing.B) {
						runUnary(b, maxC, reqS, respS, s)
						isMatched = true
					})
					if isMatched {
						fmt.Print(s.String())
						isMatched = false
					}
					b.Run(fmt.Sprintf("Stream-%s-maxConcurrentCalls_"+
							"%#v-reqSize_%#vB-respSize_%#vB", tracing, maxC, reqS, respS), func(b *testing.B) {
						runStream(b, maxC, reqS, respS, s)
						isMatched = true
					})
					if isMatched {
						fmt.Print(s.String())
						isMatched = false
					}
				}
			}
		}
	}

}

/*
func TestMain(m *testing.M) {
	os.Exit(stats.RunTestMain(m))
}
*/
