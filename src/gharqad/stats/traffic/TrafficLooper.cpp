



#include <nekobox/stats/traffic/TrafficLooper.hpp>
#include <nekobox/api/RPC.h>
#include <nekobox/global/GuiUtils.hpp>
#include <nekobox/ui/mainwindow_interface.h>

#include <QElapsedTimer>
#include <QJsonDocument>
#include <QThread>

namespace Stats {

TrafficLooper *trafficLooper = new TrafficLooper;
QElapsedTimer elapsedTimer;

#define MIGRATE_LOG(proxy, uplink) \
                Stats::databaseLogger->total_##proxy->uplink += proxy->uplink; \
                proxy->uplink = 0;

        TrafficLooper::TrafficLooper(){
            this->save_control_no_save(true);
            proxy = std::make_shared<Stats::TrafficData>("proxy");
            direct = std::make_shared<Stats::TrafficData>("direct");
        }

        void TrafficLooper::initialize(){
            if (this->save_control_no_save()){
                this->Load();
                this->save_control_no_save(false);
                MIGRATE_LOG(proxy, uplink)
                MIGRATE_LOG(proxy, downlink)
                MIGRATE_LOG(direct, uplink)
                MIGRATE_LOG(direct, downlink)
            }
        }

  
int TrafficLooper::total_direct_download(){
    return direct->downlink;
}


int TrafficLooper::total_proxy_download(){
    return proxy->downlink;
}


int TrafficLooper::total_direct_upload(){
    return direct->uplink;
}

int TrafficLooper::total_proxy_upload(){
    return proxy->uplink;
}

void TrafficLooper::UpdateAll() {
  if (Configs::dataStore->disable_traffic_stats) {
    return;
  }

  auto resp = API::defaultClient->QueryStats();
  proxy->uplink_rate = 0;
  proxy->downlink_rate = 0;
  direct->uplink_rate = 0;
  direct->downlink_rate = 0;

  int proxyUp = 0, proxyDown = 0;
  if (resp.has_value()) {
    auto & ups = resp->ups;
    auto & downs = resp->downs;

    for (const auto &item : this->items) {
      auto tag = (item->tag);
      if (!ups.contains(tag))
        continue;
      auto now = elapsedTimer.elapsed();
      auto interval = now - item->last_update;
      item->last_update = now;
      if (interval <= 0)
        continue;
      auto up = ups[tag];
      auto down = downs[tag];
      if (item->tag == "proxy") {
        proxyUp = up;
        proxyDown = down;
      }
      item->uplink += up;
      item->downlink += down;
      item->uplink_rate =
          static_cast<double>(up) * 1000.0 / static_cast<double>(interval);
      item->downlink_rate =
          static_cast<double>(down) * 1000.0 / static_cast<double>(interval);
      if (item->ignoreForRate)
        continue;

      TrafficData * data_edit = (item->tag == "direct") ? direct.get() : proxy.get();

        data_edit->uplink += up;
        data_edit->downlink += down;
        data_edit->uplink_rate += item->uplink_rate;
        data_edit->downlink_rate += item->downlink_rate;
    }
  }
  if (isChain) {
    for (const auto &item : this->items) {
      if (item->isChainTail) {
        item->uplink += proxyUp;
        item->downlink += proxyDown;
      }
    }
  }
}

void TrafficLooper::Loop() {
  elapsedTimer.start();
  while (true) {
    QThread::msleep(1000); // refresh every one second

    if (Configs::dataStore->disable_traffic_stats) {
      continue;
    }

    // profile start and stop
    if (!loop_enabled) {
      // 停止
      if (looping) {
        looping = false;
        runOnUiThread([=, this] {
          auto m = GetMainWindow();
          m->refresh_status("STOP");
          m->update_traffic_graph(0, 0, 0, 0);
        });
      }
      continue;
    } else {
      // 开始
      if (!looping) {
        looping = true;
      }
    }

    // do update
    QString summary;
    const auto epoch = generation.load();
    QList<int> profileIds;
    int proxyDl, proxyUl, directDl, directUl;
    {
      QMutexLocker lock(&loop_mutex);
      // A stop may have been queued while the previous poll owned the lock.
      if (!loop_enabled) continue;
      UpdateAll();
      if (!loop_enabled) continue;
      summary = QObject::tr("Proxy: ⚡%1 📦%3\nDirect: ⚡%2 📦%4")
                    .arg(proxy->DisplaySpeed(), direct->DisplaySpeed(),
                         proxy->DisplayTraffic(), direct->DisplayTraffic());
      proxyDl = proxy->downlink_rate;
      proxyUl = proxy->uplink_rate;
      directDl = direct->downlink_rate;
      directUl = direct->uplink_rate;
      for (const auto &item : items)
        if (item->id >= 0) profileIds.append(item->id);
    }

    // post to UI
    runOnUiThread([=, this] {
      auto m = GetMainWindow();
      if (!m || !loop_enabled || generation.load() != epoch) return;
      m->refresh_status(summary);
      m->update_traffic_graph(proxyDl, proxyUl, directDl, directUl);
      for (const int id : profileIds) m->refresh_proxy_traffic(id);
    });
  }
}

} // namespace Stats
