// +build !appengine,go1.7

/*
 *
 * Copyright 2018 gRPC authors.
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

// Package syscall provides functionalities that grpc uses to get low-level operating system
// stats/info.
package syscall

import (
	"syscall"

	"github.com/bglmmz/grpc/grpclog"
	"golang.org/x/sys/unix"
)

// GetCPUTime returns the how much CPU time has passed since the start of this process.
func GetCPUTime() int64 {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_PROCESS_CPUTIME_ID, &ts); err != nil {
		grpclog.Fatal(err)
	}
	return ts.Nano()
}

// Rusage is an alias for syscall.Rusage under linux non-appengine environment.
type Rusage syscall.Rusage

// GetRusage returns the resource usage of current process.
func GetRusage() (rusage *Rusage) {
	rusage = new(Rusage)
	syscall.Getrusage(syscall.RUSAGE_SELF, (*syscall.Rusage)(rusage))
	return
}

// CPUTimeDiff returns the differences of user CPU time and system CPU time used
// between two Rusage structs.
func CPUTimeDiff(first *Rusage, latest *Rusage) (float64, float64) {
	f := (*syscall.Rusage)(first)
	l := (*syscall.Rusage)(latest)
	var (
		utimeDiffs  = l.Utime.Sec - f.Utime.Sec
		utimeDiffus = l.Utime.Usec - f.Utime.Usec
		stimeDiffs  = l.Stime.Sec - f.Stime.Sec
		stimeDiffus = l.Stime.Usec - f.Stime.Usec
	)

	uTimeElapsed := float64(utimeDiffs) + float64(utimeDiffus)*1.0e-6
	sTimeElapsed := float64(stimeDiffs) + float64(stimeDiffus)*1.0e-6

	return uTimeElapsed, sTimeElapsed
}
