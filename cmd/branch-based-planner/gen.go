package main

import "fmt"

func leaderElectionID(prefix string) string {
	return fmt.Sprintf("%s.weaveworks.contrib.fluxcd.io", prefix)
}
