#!/usr/bin/env bash
echo $TRAVIS_GO_VERSION
if [ $TRAVIS_GO_VERSION = 1.8* ]; then
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
    git reset --hard ${commits[0]}
    ls benchmark/c/
    if [ -e "benchmark/c/main.go" ]; then
      echo "after reset: file benchmark/c/main.go exist"
      go test google.golang.org/grpc/benchmark/... -benchmem -bench=BenchmarkClient/Unary-Tracing-maxConcurrentCalls_64 | tee benchmark/c/result2
      ls
      go run benchmark/c/main.go benchmark/c/result1 benchmark/c/result2
    else
      echo "not exist"
      mv benchmark/c/tmp benchmark/c/main.go
      ls benchmark/c/
      go test google.golang.org/grpc/benchmark/... -benchmem -bench=BenchmarkClient/Unary-Tracing-maxConcurrentCalls_64 | tee benchmark/c/result2
      ls benchmark/c/
      go run benchmark/c/main.go benchmark/c/result1 benchmark/c/result2
    fi
  fi
fi
