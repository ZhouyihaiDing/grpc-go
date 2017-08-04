/*
 *
 * Copyright 2014 gRPC authors.
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

package grpc

import (
	"io"
)

var (
	// cpFunc map[string] func(io.Writer)(io.WriteCloser)
	// dcFunc map[string] func(io.Reader)(io.ReadCloser, error)
	cpFunc = make(map[string]Compressor)
	dcFunc map[string]Compressor
)

type generalCompressor struct {
	cpType string
	// only one of cp and writerFunc need to be set
	cp         Compressor
	writerFunc func(io.Writer) io.WriteCloser
}

// NewGeneralCompressor extends gzipCompressor with a "type" and "writer function",
// which can be created by user. "cp" is used for old gizp compressor.
func NewGeneralCompressor(name string, cp Compressor, f func(io.Writer) io.WriteCloser) Compressor {
	return &generalCompressor{cpType: name, cp: cp, writerFunc: f}
}

func (c *generalCompressor) getCompressWriter(w io.Writer) io.WriteCloser {
	return c.writerFunc(w)
}

func (c *generalCompressor) Write(w io.Writer, p []byte) error {
	if c.cp != nil {
		return c.cp.Do(w, p)
	}
	return nil
}

// Do function is used by old gizp compressor. For user created compressor, it's nil.
func (c *generalCompressor) Do(w io.Writer, p []byte) error {
	if c.cp != nil {
		return c.cp.Do(w, p)
	}
	return nil
}

// Keep Type() function to set s.sendCompress
func (c *generalCompressor) Type() string {
	return c.cpType
}
