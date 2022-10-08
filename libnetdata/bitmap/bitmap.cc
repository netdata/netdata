#include "bitmap.h"
#include <vector>

using BoolVector = std::vector<bool>;

bitmap_t bitmap_new(size_t capacity) {
    return new BoolVector(capacity, 0);
}

void bitmap_delete(bitmap_t bitmap) {
    BoolVector *BV = static_cast<BoolVector *>(bitmap);
    delete BV;
}

void bitmap_set(bitmap_t bitmap, size_t index, bool value) {
    BoolVector *BV = static_cast<BoolVector *>(bitmap);
    (*BV)[index] = value;
}

bool bitmap_get(bitmap_t bitmap, size_t index) {
    BoolVector *BV = static_cast<BoolVector *>(bitmap);
    return (*BV)[index];
}
