// Copyright 2020 Envoyproxy Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.
package logger

import (
	"log"
)

// An example of a logger that implements `pkg/log/Std`.  Logs to
// stdout.  If Debug == false then Debugf() and Infof() won't output
// anything.
type Std struct {
	Debug bool
}

// Log to stdout only if Debug is true.
func (l *Std) Debugf(format string, args ...interface{}) {
	if l.Debug {
		log.Printf(format+"\n", args...)
	}
}

// Log to stdout only if Debug is true.
func (l *Std) Infof(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}

// Log to stdout always.
func (l *Std) Warnf(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}

// Log to stdout always.
func (l *Std) Errorf(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}

func (l *Std) Fatalf(format string, args ...interface{}) {
	log.Fatalf(format+"\n", args...)
}
