#ifndef ML_STRING_WRAPPER_H
#define ML_STRING_WRAPPER_H

#include "libnetdata/libnetdata.h"

#include <algorithm>

class StringWrapper {
public:
    StringWrapper() noexcept : Inner(nullptr)
    {
    }

    explicit StringWrapper(const char *S) noexcept : Inner(string_strdupz(S))
    {
    }

    explicit StringWrapper(STRING *S) noexcept : Inner(string_dup(S))
    {
    }

    StringWrapper(const StringWrapper &Other) noexcept : Inner(string_dup(Other.Inner))
    {
    }

    StringWrapper &operator=(const StringWrapper &Other) noexcept
    {
        if (this != &Other) {
            STRING *Tmp = string_dup(Other.Inner);
            string_freez(Inner);
            Inner = Tmp;
        }
        return *this;
    }

    StringWrapper(StringWrapper &&Other) noexcept : Inner(Other.Inner)
    {
        Other.Inner = nullptr;
    }

    StringWrapper &operator=(StringWrapper &&Other) noexcept
    {
        if (this != &Other) {
            string_freez(Inner);
            Inner = Other.Inner;
            Other.Inner = nullptr;
        }
        return *this;
    }

    ~StringWrapper()
    {
        string_freez(Inner);
    }

    STRING *inner() const noexcept
    {
        return Inner;
    }

    operator const char *() const noexcept
    {
        return string2str(Inner);
    }

    void swap(StringWrapper &Other) noexcept
    {
        std::swap(Inner, Other.Inner);
    }

private:
    STRING *Inner;
};

// Free swap function
inline void swap(StringWrapper &LHS, StringWrapper &RHS) noexcept
{
    LHS.swap(RHS);
}

#endif /* ML_STRING_WRAPPER_H */
