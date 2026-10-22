#ifdef NDEBUG
#undef NDEBUG
#endif
#include <nekobox/api/ResponsiveTask.hpp>
#include <QTimer>
#include <QThread>
#include <cassert>
#include <stdexcept>

int main(int argc, char **argv) {
    QCoreApplication app(argc, argv);
    int ticks = 0;
    QTimer heartbeat;
    QObject::connect(&heartbeat, &QTimer::timeout, &app, [&] { ++ticks; });
    heartbeat.start(5);
    const auto guiThread = QThread::currentThread();
    const int result = API::responsiveTask([guiThread] {
        assert(QThread::currentThread() != guiThread);
        QThread::msleep(100);
        return 42;
    });
    assert(result == 42 && ticks >= 2); // Slow RPC must not stall GUI timers.
    for (int i = 0; i < 100; ++i) {
        assert(API::responsiveTask([i] { return i; }) == i);
    }
    bool caught = false;
    try { API::responsiveTask([]() -> int { throw std::runtime_error("test failure"); }); }
    catch (...) { caught = true; }
    assert(caught);
}
