VMLINUX_H ?= vmlinux.h
BPF_OBJ ?= rewind_trace.bpf.o
BPF_ARCH ?= arm64
BPF_MULTIARCH ?= $(shell dpkg-architecture -qDEB_HOST_MULTIARCH 2>/dev/null)

BPF_INCLUDE_FLAGS := -I.
ifneq ($(strip $(BPF_MULTIARCH)),)
BPF_INCLUDE_FLAGS += -I/usr/include/$(BPF_MULTIARCH)
endif

.PHONY: vmlinux compile clean

vmlinux:
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > $(VMLINUX_H)

compile: $(VMLINUX_H)
	clang -O2 -g -target bpf -D__TARGET_ARCH_$(BPF_ARCH) $(BPF_INCLUDE_FLAGS) -c rewind_trace.bpf.c -o $(BPF_OBJ)

clean:
	rm -f $(VMLINUX_H) $(BPF_OBJ)
