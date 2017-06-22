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
if [ -d "benchmark/c" ]; then
  echo "dir benchmark/c exist"
  cp benchmark/c/main.go benchmark/c/tmp
  go test google.golang.org/grpc/benchmark/... -benchmem -bench=BenchmarkClient/Unary-Tracing-maxConcurrentCalls_64 | tee benchmark/c/result1
  ls benchmark/c/
  cat benchmark/c/result1
  git reset --hard ${commits[0]}
  ls benchmark/c/
  if [ -e "benchmark/c/main.go" ]; then
    echo "after reset: dir benchmark/c exist"
    go test google.golang.org/grpc/benchmark/... -benchmem -bench=BenchmarkClient/Unary-Tracing-maxConcurrentCalls_64 | tee benchmark/compare/result2
    ls
    go run benchmark/compare/main.go benchmark/compare/result1 benchmark/compare/result2
  else
    echo "not exist"
    mv benchmark/c/tmp benchmark/c/main.go
    ls
    go test google.golang.org/grpc/benchmark/... -benchmem -bench=BenchmarkClient/Unary-Tracing-maxConcurrentCalls_64 | tee benchmark/compare/result2
    ls
    go run benchmark/c/main.go benchmark/c/result1 benchmark/c/result2
  fi
fi
