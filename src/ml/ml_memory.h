// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_MEMORY_H
#define NETDATA_ML_MEMORY_H

// ml_alloc_active gates the global operator new/delete overrides defined in
// ml_memory.cc. The override unconditionally calls malloc/free, but only
// updates the pulse_ml_memory_* counters when this flag is true on the
// calling thread.
//
// The flag is set by MlAllocScope at the top of every public ML entry
// point, so allocations triggered from inside ML code count toward the ML
// memory line on netdata.memory, while allocations performed by unrelated
// C++ code (notably ACLK protobuf serialization) are ignored.
//
// The variable is defined in ml.cc so it exists in every ENABLE_ML build,
// including mimalloc builds where ml_memory.cc is not compiled.
extern thread_local bool ml_alloc_active;

// MlAllocScope is a reentrant RAII guard: the constructor saves the
// previous value and sets the flag to true; the destructor restores the
// previous value. Nested scopes are harmless.
struct MlAllocScope {
    bool prev_;
    MlAllocScope() : prev_(ml_alloc_active) { ml_alloc_active = true; }
    ~MlAllocScope() { ml_alloc_active = prev_; }
    MlAllocScope(const MlAllocScope &) = delete;
    MlAllocScope &operator=(const MlAllocScope &) = delete;
};

#endif /* NETDATA_ML_MEMORY_H */
