#pragma once

#include <QCoreApplication>
#include <QEventLoop>
#include <QThread>
#include <future>
#include <memory>
#include <utility>

namespace API {
// Legacy settings APIs return a synchronous result. Run their bounded I/O on a
// worker while still processing paint, timers and core-exit notifications.
// Normal profile transitions use fully asynchronous watchers instead.
template <typename Function>
auto responsiveTask(Function function) {
    using Result = decltype(function());
    const auto app = QCoreApplication::instance();
    if (!app || QThread::currentThread() != app->thread()) return function();
    auto task = std::make_shared<std::packaged_task<Result()>>(std::move(function));
    auto future = task->get_future();
    QEventLoop loop;
    auto worker = QThread::create([task] { (*task)(); });
    QObject::connect(worker, &QThread::finished, &loop, &QEventLoop::quit);
    QObject::connect(worker, &QThread::finished, worker, &QObject::deleteLater);
    worker->start();
    if (future.wait_for(std::chrono::seconds(0)) != std::future_status::ready)
        loop.exec(QEventLoop::ExcludeUserInputEvents);
    return future.get();
}
}
