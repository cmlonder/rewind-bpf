VMLINUX_H ?= vmlinux.h
BPF_OBJ ?= rewind_trace.bpf.o
BPF_ARCH ?= arm64

.PHONY: vmlinux compile clean

vmlinux:
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > $(VMLINUX_H)

compile: $(VMLINUX_H)
	clang -O2 -g -target bpf -D__TARGET_ARCH_$(BPF_ARCH) -I. -c rewind_trace.bpf.c -o $(BPF_OBJ)

clean:
	rm -f $(VMLINUX_H) $(BPF_OBJ)
