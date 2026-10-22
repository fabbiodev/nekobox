#pragma once

#include <algorithm>
#include <deque>
#include <functional>
#include <optional>

// Owned exclusively by the GUI thread. RPC workers never mutate this queue.
struct ProfileOperation {
    int id = -1; // Negative means stop only.
    bool doNotTest = true;
    bool crash = false;
    bool manual = false;
    std::function<void()> completed;
};

class ProfileOperationQueue {
public:
    void enqueue(ProfileOperation operation) {
        // Only the latest requested start matters. Explicit stops are retained,
        // including their completion callbacks (shutdown may be waiting on one).
        std::erase_if(pending, [](const auto &item) { return item.id >= 0; });
        pending.push_back(std::move(operation));
    }

    std::optional<ProfileOperation> next() {
        if (active || pending.empty()) return std::nullopt;
        active = true;
        auto operation = std::move(pending.front());
        pending.pop_front();
        return operation;
    }

    void finish() { active = false; }
    bool hasPending() const { return !pending.empty(); }

private:
    bool active = false;
    std::deque<ProfileOperation> pending;
};
