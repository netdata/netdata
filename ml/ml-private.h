// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_PRIVATE_H
#define ML_PRIVATE_H

#include "kmeans/KMeans.h"
#include "ml/ml.h"

#include <chrono>
#include <map>
#include <mutex>
#include <sstream>

namespace ml {

using SteadyClock = std::chrono::steady_clock;
using TimePoint = std::chrono::time_point<SteadyClock>;

template<typename T>
using Duration = std::chrono::duration<T>;

using Seconds = std::chrono::seconds;

} // namespace ml

#endif /* ML_PRIVATE_H */
