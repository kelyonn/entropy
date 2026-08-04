#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "Dual MIT/GPL";

enum event_kind {
    EVENT_PROCESS_KILL = 1,
    EVENT_PROCESS_EXIT = 2,
    EVENT_OOM_KILL = 3,
    EVENT_KERNEL_PANIC = 4,
};

struct kernel_event {
    __u32 pid;
    __s32 value;
    __u32 kind;
    __u32 reserved;
};

struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
} events SEC(".maps");

struct trace_event_raw_sys_enter {
    __u64 unused;
    long syscall_nr;
    unsigned long args[6];
};

static __always_inline int emit_event(void *ctx, __u32 pid, __s32 value, __u32 kind)
{
    struct kernel_event event = {
        .pid = pid,
        .value = value,
        .kind = kind,
    };
    return bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &event, sizeof(event));
}

SEC("tracepoint/syscalls/sys_enter_kill")
int trace_sys_enter_kill(struct trace_event_raw_sys_enter *ctx)
{
    __u32 target_pid = (__u32)ctx->args[0];
    if (target_pid != 1)
        return 0;
    return emit_event(ctx, target_pid, (__s32)ctx->args[1], EVENT_PROCESS_KILL);
}

SEC("tracepoint/syscalls/sys_enter_exit_group")
int trace_sys_exit_group(struct trace_event_raw_sys_enter *ctx)
{
    __u32 pid = (__u32)(bpf_get_current_pid_tgid() >> 32);
    __s32 exit_code = (__s32)ctx->args[0];
    if (exit_code == 0)
        return 0;
    return emit_event(ctx, pid, exit_code, EVENT_PROCESS_EXIT);
}

SEC("kprobe/oom_kill_process")
int trace_oom_kill_process(struct pt_regs *ctx)
{
    __u32 pid = (__u32)(bpf_get_current_pid_tgid() >> 32);
    return emit_event(ctx, pid, 137, EVENT_OOM_KILL);
}

SEC("kprobe/panic")
int trace_kernel_panic(struct pt_regs *ctx)
{
    __u32 pid = (__u32)(bpf_get_current_pid_tgid() >> 32);
    return emit_event(ctx, pid, 99, EVENT_KERNEL_PANIC);
}
