// Copyright 2026 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGoCommandCancellation(t *testing.T) {
	dir := t.TempDir()
	// Model the Go process and its compiler child. Readiness is tied to that
	// child's PID; the long sleep is blocked work, not test synchronization.
	script := "#!/bin/sh\nsleep 600 &\necho $!\nwait\n"
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := goCommand(ctx, options{dir: dir})
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || pid <= 1 {
		t.Fatalf("invalid child readiness %q: %v", line, err)
	}
	cancel()
	select {
	case <-done:
		if waitErr == nil {
			t.Fatal("cancelled command passed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled command was not reaped")
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		// A killed grandchild may await reaping by init, but must not run.
		_, state, _ := strings.Cut(string(stat), ") ")
		if strings.HasPrefix(state, "Z ") || strings.HasPrefix(state, "X ") {
			return
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("compiler child survived cancellation: %s", stat)
		}
	}
}
