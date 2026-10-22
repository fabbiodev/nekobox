



#pragma once
#include <QMutex>
#include <QString>
#include <atomic>

namespace Stats
{
    constexpr int IDKEY = 242315;

    enum ConnectionSort
    {
        Default,
        ByDownload,
        ByUpload,
        ByProcess,
        ByTraffic,
        ByOutbound,
        ByProtocol
    };

    class ConnectionMetadata
    {
        public:
        QString id;
        long long createdAtMs;
        long long upload;
        long long download;
        QString outbound;
        QString network;
        QString dest;
        QString protocol;
        QString domain;
        QString process;
    };

    class ConnectionLister
    {
    public:
        ConnectionLister();

        void setSuspended(bool value) { suspend.store(value); ++generation; }

        // Rebuilding the table costs more than the data is worth while the user
        // is looking at another tab, so the loop idles until it is shown again.
        std::atomic<bool> tab_visible = false;

        void Loop();

        void ForceUpdate();

        void stopLoop();

        void setSort(ConnectionSort newSort);

    private:
        void update();

        QMutex mu;
        QMutex sort_mu;
        std::atomic<bool> suspend = true;
        std::atomic<uint64_t> generation{0};
        uint64_t lastGeneration = 0;

        std::atomic<bool> stop = false;

        std::shared_ptr<QSet<QString>> state;

        ConnectionSort sort = Default;

        bool asc = false;
    };

    extern ConnectionLister* connection_lister;
}
