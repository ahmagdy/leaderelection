.PHONY: run test

run:
	go run .
test:
	go test ./...

mockgen:
	rm -rf mocks && mockgen -source ./leader_election.go -destination  mocks/leader_election.go
