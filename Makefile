.PHONY: run test

run:
	go run .
test:
	go test -v ./...

mockgen:
	rm -rf mocks && mockgen -source ./leader_election.go -destination  mocks/leader_election.go
