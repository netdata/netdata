#include <cstddef>
#include <cstdint>
#include <cstring>
#include <vector>
#include <cassert>

// Function declaration
extern "C" size_t quoted_strings_splitter_pluginsd_re2c(char *start, char **words, size_t max_words);


extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size)
{
    if (Size == 0)
        return 0;

    const size_t MAX_WORDS = 5;

    std::vector<char> buffer(Size + 1);
    std::memcpy(buffer.data(), Data, Size);
    buffer[Size] = '\0'; // Ensure null-termination

    // Create array to store word pointers
    std::vector<char *> words(MAX_WORDS, nullptr);

    // Run the splitter
    size_t word_count = quoted_strings_splitter_pluginsd_re2c(buffer.data(), words.data(), MAX_WORDS);

    // Basic invariant checks
    assert(word_count <= MAX_WORDS && "Returned more words than max_words");

    // Verify all returned word pointers are within our buffer
    for (size_t i = 0; i < word_count; i++) {
        if (words[i]) {
            assert(
                words[i] >= buffer.data() && words[i] < (buffer.data() + buffer.size()) &&
                "Word pointer outside buffer bounds");

            // Verify null-termination of each word
            assert(strlen(words[i]) < buffer.size() && "Word not properly null-terminated");
        }
    }

    // Verify remaining array elements are null
    for (size_t i = word_count; i < MAX_WORDS; i++) {
        assert(words[i] == nullptr && "Non-null pointer beyond word_count");
    }

    return 0;
}
