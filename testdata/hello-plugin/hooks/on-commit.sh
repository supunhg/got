#!/bin/bash
# Sample GOT plugin hook: fires when a CommitCreated event is published.
# Reads event JSON from stdin and prints it for debugging.
echo "Hello from hello-world plugin! A commit was created."
cat - | head -c 500
echo ""
