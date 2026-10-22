#ifdef NDEBUG
#undef NDEBUG
#endif
#include <nekobox/ui/utils/ProfileOperationQueue.h>
#include <cassert>

int main() {
    ProfileOperationQueue queue;
    queue.enqueue({1});
    assert(queue.next()->id == 1);
    assert(!queue.next()); // Never start two workers together.
    for (int id = 2; id <= 1000; ++id) queue.enqueue({id});
    assert(!queue.next());
    queue.finish();
    assert(queue.next()->id == 1000); // Last selection wins.
    queue.finish();
    assert(!queue.next());

    int stopped = 0;
    queue.enqueue({2});
    queue.enqueue({-1, true, false, true, [&] { ++stopped; }});
    queue.enqueue({3});
    auto stop = queue.next();
    assert(stop && stop->id < 0); // Stop cancels a pending start, not vice versa.
    stop->completed();
    queue.finish();
    assert(queue.next()->id == 3);
    queue.finish();
    assert(stopped == 1 && !queue.next());

    queue.enqueue({4});
    assert(queue.next()->id == 4);
    queue.enqueue({5});
    queue.enqueue({-1, true, true, false, [&] { ++stopped; }});
    queue.finish();
    stop = queue.next();
    assert(stop && stop->crash && stop->id < 0);
    stop->completed();
    queue.finish();
    assert(stopped == 2 && !queue.next());
}
