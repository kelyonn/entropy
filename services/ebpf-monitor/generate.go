package main

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -Werror" entropybpf bpf/entropy.bpf.c -- -I/usr/include
