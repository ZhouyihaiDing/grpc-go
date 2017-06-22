#!/usr/bin/env bash
make testdeps
sleep 1
echo $TRAVIS_PULL_REQUEST
echo $TRAVIS_COMMIT_RANGE
IFS='...' read -r -a commits <<< "$TRAVIS_COMMIT_RANGE"
echo "base commit number"
echo ${commits[0]}
echo "current commit number"
echo ${commits[-1]}
if [ -d "benchmark/compare" ]; then
  echo "dir benchmark/compare exist"
  go test google.golang.org/grpc/benchmark/... -benchmem -bench=BenchmarkClient/Unary-Tracing-maxConcurrentCalls_64 | tee benchmark/compare/result1
  ls benchmark/compare/
  cat benchmark/compare/result1
  git reset --hard ${commits[0]}
  ls benchmark/compare/
  if [ -d "benchmark/compare" ]; then
    echo "after reset: dir benchmark/compare exist"
    go test google.golang.org/grpc/benchmark/... -benchmem -bench=BenchmarkClient/BenchmarkClient/Unary-Tracing-maxConcurrentCalls_64 | tee benchmark/compare/result2
    ls
    go run benchmark/compare/main.go benchmark/compare/result1 benchmark/compare/result2
  fi
fi
