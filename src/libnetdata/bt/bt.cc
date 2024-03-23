#include "bt.h"

#include <backtrace.h>
#include <backtrace-supported.h>

#include <algorithm>
#include <cstdio>
#include <fstream>
#include <mutex>
#include <sstream>
#include <unordered_map>
#include <queue>

static backtrace_state *State = nullptr;

static int pcinfo_callback(void *data, uintptr_t pc, const char *filename, int lineno, const char *function)
{
    std::ostringstream *OS = static_cast<std::ostringstream*>(data);

    if (function)
        *OS << function << "() @ ";

    if (filename)
        *OS << filename << ":" << lineno;
    else
        *OS << pc << " (information not available)";

    *OS << "\n";
    return 0;
}

static void error_callback(void *data, const char *msg, int errnum)
{
    std::ostringstream *OS = static_cast<std::ostringstream*>(data);
    *OS << "Backtrace error: " << msg << " (error number " << errnum << ")\n";
}

struct UuidKey
{
    const uuid_t *Inner;

    bool operator==(const UuidKey& Other) const
    {
        return uuid_compare(*Inner, *Other.Inner) == 0;
    }
};

namespace std
{
    template<>
    struct hash<UuidKey>
    {
        size_t operator()(const UuidKey& Key) const
        {
            return XXH64(*Key.Inner, sizeof(uuid_t), 0);
        }
    };
}

class StackTrace
{
public:
    static const size_t MAX_ITEMS = 128;
    uintptr_t PCs[MAX_ITEMS] = { 0 };
    size_t Items = 0;

    void append(uintptr_t PC)
    {
        assert(Items < MAX_ITEMS);
        PCs[Items++] = PC;
    }

    bool operator==(const StackTrace& Other) const
    {
        if (Items != Other.Items)
            return false;

        for (size_t i = 0; i < Items; i++)
            if (PCs[i] != Other.PCs[i])
                return false;

        return true;
    }

    void dump(std::ostream &OS) const
    {
        for (size_t i = 0; i < Items; ++i)
            backtrace_pcinfo(State, PCs[i], pcinfo_callback, error_callback, &OS);
        OS << std::endl;
    }
};

namespace std
{
    template<>
    struct hash<StackTrace>
    {
        size_t operator()(const StackTrace& ST) const
        {
            return XXH64(ST.PCs, ST.Items * sizeof(uintptr_t), 0);
        }
    };
}

static std::vector<std::pair<uint64_t, StackTrace>> InternedStackTraces;

static size_t stackTraceID(const StackTrace &ST)
{
    std::hash<StackTrace> hasher;
    uint64_t K = hasher(ST);

    auto Pred = [](const std::pair<uint64_t, StackTrace>& a, const std::pair<uint64_t, StackTrace>& b) {
        return a.first < b.first;
    };

    std::pair<uint64_t, StackTrace> P(K, ST);
    auto It = std::lower_bound(InternedStackTraces.begin(), InternedStackTraces.end(), P, Pred);
    if (It != InternedStackTraces.end() && It->first == K)
        return K;

    InternedStackTraces.insert(It, {K, ST});
    return K;
}

static const StackTrace &lookupStackTrace(uint64_t ID)
{
    auto Pred =  [](const std::pair<uint64_t, StackTrace>& element, uint64_t value) {
        return element.first < value;
    };
    auto It = std::lower_bound(InternedStackTraces.begin(), InternedStackTraces.end(), ID, Pred);

    return It->second;
}

static std::unordered_map<UuidKey, std::queue<uint64_t>> USTs;
static std::mutex Mutex;

static int simple_callback(void *data, uintptr_t pc)
{
    StackTrace *ST = static_cast<StackTrace*>(data);
    if (ST->Items == StackTrace::MAX_ITEMS)
        fatal("StackTrace too big...");

    ST->append(pc);
    return 0;
}

const char *bt_path = NULL;

void bt_init(const char *exepath, const char *cache_dir)
{
    State = backtrace_create_state(exepath, 1, nullptr, nullptr);

    char buf[FILENAME_MAX + 1];
    snprintfz(buf, FILENAME_MAX, "%s/%s", cache_dir, "bt.log");
    bt_path = strdupz(buf);
}

void bt_collect(const uuid_t *uuid)
{
    // Enable collection on 1/16th of UUIDs to save on CPU and RAM consumption
    if (*uuid[0] != 0x0A)
        return;

    {
        std::lock_guard<std::mutex> lock(Mutex);

        UuidKey UK = { uuid };

        auto& Q = USTs[UK];
        if (Q.size() == 128)
            Q.pop();

        StackTrace ST;
        backtrace_simple(State, 1, simple_callback, error_callback, &ST);
        Q.push(stackTraceID(ST));
    }
}

void bt_dump(const uuid_t *uuid)
{
    std::lock_guard<std::mutex> lock(Mutex);

    UuidKey UK = { uuid };

    auto It = USTs.find(UK);
    if (It == USTs.end())
        return;

    std::queue<uint64_t> Q = It->second;
    std::ostringstream OS;

    size_t Idx = 0;
    while (!Q.empty())
    {
        OS << "Stack trace " << ++Idx << "/" << It->second.size() << ":\n";
        const StackTrace& ST = lookupStackTrace(Q.front());
        ST.dump(OS);
        Q.pop();
    }

    std::ofstream OF{bt_path};
    if (OF.is_open())
    {
        OF << OS.str();
        OF.close();
    }
}
