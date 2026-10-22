

#include "libcore_types.h"
#include <nekobox/dataStore/ProxyEntity.hpp>
#include <nekobox/ui/mainwindow.h>
#include <nekobox/dataStore/Database.hpp>
#include <nekobox/configs/ConfigBuilder.hpp>
#include <nekobox/sys/Settings.h>
#include <nekobox/dataStore/Utils.hpp>
#include <nekobox/stats/traffic/TrafficLooper.hpp>
#include <nekobox/api/RPC.h>
#include <nekobox/ui/utils/MessageBoxTimer.h>
#include <3rdparty/qv2ray/v2/proxy/QvProxyConfigurator.hpp>
#include <nekobox/global/GuiUtils.hpp>
#include <QInputDialog>
#include <QCoreApplication>
#include <QFileInfo>
#include <QPushButton>
#include <QDesktopServices>
#include <QMessageBox>
#include <QSemaphore>
#include <QStringList>
#include <QCoreApplication>
#include <QEventLoop>
#include <QThread>
#include <QFutureWatcher>
#include <QScopeGuard>
#include <QTimer>
#include <exception>


#ifndef NKR_SOFTWARE_KEYS
#define ADD_SECURITY_ACTION
#define CHECK_SETTINGS_ACCESS_W
#define CHECK_SETTINGS_ACCESS
#define CHECK_ACTION_ACCESS_W
#define CHECK_ACTION_ACCESS
#else
#include <nekobox/ui/security_addon.h>
#endif


// rpc
using namespace API;

void MainWindow::setup_rpc() {
    // Setup Connection
    defaultClient = std::make_unique<Client>();

    // Looper
    runOnNewThread([=] { Stats::trafficLooper->Loop(); });
    runOnNewThread([=] { Stats::connection_lister->Loop(); });

    // Start auto-testing if enabled (access via GetMainWindow since this is static)
    auto mw = GetMainWindow();
    if (Configs::dataStore->auto_test_enable && mw && mw->proxyAutoTester) {
        mw->proxyAutoTester->Start();
        MW_show_log("[Auto-Test] Started with interval of " +
                    QString::number(Configs::dataStore->auto_test_interval_seconds) + " seconds");
    }
}

void MainWindow::clear_ruleset_cache(){
    bool isok;
    libcore::CacheURLRequest req;
    req.filepath = false;
    req.clear = true;
    req.use_default_outbound = false;
    req.http_url = "";
    defaultClient->CacheHTTP(&isok, req);
}


bool MainWindow::fetch_ruleset_cache(const QString & url){
    bool isok;
    libcore::CacheURLRequest req;
    req.clear = false;
    req.filepath = false;
    req.use_default_outbound = Configs::dataStore->network_use_proxy;
    req.http_url = url.toStdString();
    defaultClient->CacheHTTP(&isok, req);
    return isok;
}

void MainWindow::runURLTest(const QString& config, bool useDefault, const QStringList& outboundTags, const QMap<QString, int>& tag2entID, int entID) {
    if (stopSpeedtest.load()) {
        MW_show_log(tr("Profile test aborted"));
        return;
    }

    libcore::TestReq req;
    req.outbound_tags = QListStr2VectorStr(outboundTags);

    req.config = (config.toStdString());
    req.url = (Configs::dataStore->test_latency_url.toStdString());
    req.use_default_outbound = (useDefault);
    req.max_concurrency = (Configs::dataStore->test_concurrent);
    req.test_timeout_ms = (Configs::dataStore->url_test_timeout_ms);

    const auto done = std::make_shared<std::atomic<bool>>(false);
    const auto pollingDone = std::make_shared<QSemaphore>();
    runOnNewThread([=,this]
    {
        const auto complete = qScopeGuard([pollingDone] { pollingDone->release(); });
        bool ok;
        while (true)
        {
            QThread::msleep(200);
            if (done->load()) break;
            auto resp = defaultClient->QueryURLTest(&ok);
            if (!ok || resp->results.empty() )
            {
                continue;
            }

            bool needRefresh = false;
            for (const auto& res : resp->results)
            {
                int entid = -1;
                if (!tag2entID.isEmpty()) {
                    auto tag = QString::fromUtf8(res.outbound_tag.c_str());
                    entid = tag2entID.count(tag) == 0 ? -1 : tag2entID[tag];
                }
                if (entid == -1) {
                    continue;
                }
                std::shared_ptr<Configs::ProxyEntity> ent = Configs::profileManager->GetProfile(entid);
                if (ent == nullptr) {
                    continue;
                }
                auto error = QString::fromUtf8(res.error.c_str());
                if (error.isEmpty()) {
                    ent->latencyInt = res.latency_ms;
                } else {
                    if (error.contains("test aborted") ||
                        error.contains("context canceled")) ent->latencyInt=0;
                    else {
                        ent->latencyInt = -1;
                        MW_show_log(tr("[%1] test error: %2").arg(
                            ent->DisplayTypeAndName(), error));
                    }
                }
                ent->Save();
                needRefresh = true;
            }
            if (needRefresh)
            {
                runOnUiThread([=,this]{
                    refresh_proxy_list();
                });
            }
        }
    });
    bool rpcOK;
    auto result = defaultClient->Test(&rpcOK, req);
    done->store(true);
    pollingDone->acquire();
    //
    if (!rpcOK || result->results.empty()) return;

    for (const auto &res: result->results) {
        if (!tag2entID.isEmpty()) {
            auto tag = QString::fromUtf8(res.outbound_tag.c_str());
            entID = tag2entID.count(tag) == 0 ? -1 : tag2entID[tag];
        }
        if (entID == -1) {
            MW_show_log(tr("Something is very wrong, the subject ent cannot be found!"));
            continue;
        }

        auto ent = Configs::profileManager->GetProfile(entID);
        if (ent == nullptr) {
            MW_show_log(tr("Profile manager data is corrupted, try again."));
            continue;
        }
        auto error = QString::fromUtf8(res.error.c_str());
        if (error.isEmpty()) {
            ent->latencyInt = res.latency_ms;
        } else {
            if (error.contains("test aborted") ||
                error.contains("context canceled")) ent->latencyInt=0;
            else {
                ent->latencyInt = -1;
                MW_show_log(tr("[%1] test error: %2").arg(
                ent->DisplayTypeAndName(), error));
            }
        }
        ent->Save();
    }
}

bool MainWindow::urltest_profile(std::shared_ptr<Configs::ProxyEntity> entity,
        bool skip_last_url_test_warning, const std::function<void(const QList<int>&)> &finish){
    return urltest_current_group({entity->Id()}, skip_last_url_test_warning, finish);
}

bool MainWindow::testProfileLatency(
    const std::shared_ptr<Configs::ProxyEntity> &entity,
    const std::function<void(const QList<int>&)> &finish) {
    if (entity != nullptr)
        return urltest_profile(entity, true, finish);
    return false;
}

bool MainWindow::urltest_current_group(const QList<int>& ids,
        bool skip_last_url_test_warning, const std::function<void(const QList<int>&)> &finish) {
    if (ids.isEmpty()) return false;
    if (!speedtestRunning.tryAcquire()) {
        if (!skip_last_url_test_warning)
            runOnUiThread([this] {
                QMessageBox::warning(this, software_name, tr("The last url test did not exit completely, please wait. If it persists, please restart the program."));
            });
        return false;
    }
    stopSpeedtest.store(false);
    runOnNewThread([this, ids, finish] {
        const auto release = qScopeGuard([this] { speedtestRunning.release(); });
        bool failed = false;
        for (int offset = 0; offset < ids.size() && !stopSpeedtest.load(); offset += 25) {
            QList<std::shared_ptr<Configs::ProxyEntity>> profiles;
            for (int i = offset; i < qMin(offset + 25, int(ids.size())); ++i) {
                if (auto profile = Configs::profileManager->GetProfile(ids[i])) profiles.append(profile);
            }
            if (profiles.isEmpty()) continue;
            const auto config = Configs::BuildTestConfig(profiles);
            if (!config->error.isEmpty()) {
                MW_show_log(tr("Failed to build test config: ") + config->error);
                failed = true;
                break;
            }
            // Separate per-batch completion from the global test gate. The old
            // gate was unlocked from other threads and could deadlock after 25.
            const auto done = std::make_shared<QSemaphore>();
            int jobs = 0;
            if (!config->fullConfigs.isEmpty()) {
                ++jobs;
                parallelCoreCallPool->start([this, config, done] {
                    const auto complete = qScopeGuard([done] { done->release(); });
                    for (auto it = config->fullConfigs.cbegin(); it != config->fullConfigs.cend(); ++it) {
                        if (stopSpeedtest.load()) break;
                        runURLTest(it.value(), true, {}, {}, it.key());
                    }
                });
            }
            if (!config->outboundTags.isEmpty()) {
                ++jobs;
                parallelCoreCallPool->start([this, config, done] {
                    const auto complete = qScopeGuard([done] { done->release(); });
                    if (!stopSpeedtest.load())
                        runURLTest(QJsonObject2QString(config->coreConfig, false), false,
                                   config->outboundTags, config->tag2entID);
                });
            }
            done->acquire(jobs); // Worker only; no GUI wait, no stack captures.
        }
        runOnUiThread([this] {
            refresh_proxy_list();
            MW_show_log(tr("URL test finished!"));
        });
        if (finish) finish(failed ? QList<int>{} : ids);
    });
    return true;
}

void MainWindow::stopTests() {
    stopSpeedtest.store(true);
    runOnNewThread([] {
        bool ok = false;
        API::defaultClient->StopTests(&ok);
        if (!ok) MW_show_log(MainWindow::tr("Failed to stop tests"));
    });
}

void MainWindow::url_test_current() {
    const auto testedProfile = running;
    if (!testedProfile) return;
    const auto generation = profile_generation.load();
    last_test_time = QDateTime::currentSecsSinceEpoch();
    ui->label_running->setText(tr("Testing"));

    // untagged the core would measure route.final, not the profile
    bool useProxyTag = false;
    if (running != nullptr) {
        auto profile = Configs::profileManager->GetProfile(running->id);
        useProxyTag = profile != nullptr && !profile->IsFullConfig();
    }

    runOnNewThread([=,this] {
        libcore::TestReq req;
        req.test_current = (true);
        if (useProxyTag) req.outbound_tags.push_back("proxy");
        req.url = (Configs::dataStore->test_latency_url.toStdString());

        bool rpcOK;
        auto result = defaultClient->Test(&rpcOK, req);
        if (!rpcOK || result->results.empty() ) return;

        auto results_0 = result->results.at(0);
        auto latency = results_0.latency_ms;
        runOnUiThread([=,this] {
            if (running != testedProfile || profile_generation.load() != generation) return;
            last_test_time = QDateTime::currentSecsSinceEpoch();
            if (!results_0.error.empty()) {
                MW_show_log(QString("UrlTest error: %1").arg(
                    QString::fromUtf8(results_0.error.c_str())));
            }
            auto profile = Configs::profileManager->GetProfile(running->id);
            if (profile != nullptr){
                if (latency <= 0) {
                    ui->label_running->setText(tr("Test Result") + ": " + tr("Unavailable"));
                    profile->latencyInt = -1;
                } else if (latency > 0) {
                    ui->label_running->setText(tr("Test Result") + ": " + 
                        QString::number(latency) + QString(" ms"));
                    profile->latencyInt = latency;
                }
                profile->Save();
                refresh_proxy_list(running->id);
            }
        });
    });
}

void MainWindow::speedtest_current_group(const QList<int>& profiles_ids, 
    bool testCurrent, int testmode)
{
    if (profiles_ids.isEmpty() && !testCurrent) {
        return;
    }
    if (!speedtestRunning.tryAcquire()) {
        runOnUiThread([this](){
            QMessageBox::warning(this, software_name, tr("The last speed test did not exit completely, please wait. If it persists, please restart the program."));
        });
        return;
    }

    live_test_profile_id = testCurrent && running ? running->id : -1;
    live_test_profile_generation = profile_generation.load();
    QStringList liveTags;
    if (testCurrent && running && !running->IsFullConfig()) liveTags << "proxy";
    runOnNewThread([testCurrent, this, testmode, profiles_ids, liveTags]() {
        const auto release = qScopeGuard([this] { speedtestRunning.release(); });
        if (!testCurrent)
        {
        QList<std::shared_ptr<Configs::ProxyEntity>> profiles;

        int count = profiles_ids.count();
        stopSpeedtest.store(false);
        int index = 0;
        int batch = 0;
        loop_back:
        while (index < count){
            auto profile = Configs::profileManager->GetProfile(profiles_ids.at(index));
            index ++;
            if (profile != nullptr){
             profiles << profile;
            } else {
                continue;
            }
            batch ++;
            if (batch == 25){
                break;
            }
        }
        if (!stopSpeedtest.load()){
            auto buildObject = Configs::BuildTestConfig(profiles);
            if (!buildObject->error.isEmpty()) {
                MW_show_log(tr("Failed to build test config: ") + buildObject->error);
                goto skip_loopback;
            }

            stopSpeedtest.store(false);
            for (const auto &entID: buildObject->fullConfigs.keys()) {
                auto configStr = buildObject->fullConfigs[entID];
                runSpeedTest(configStr, true, false, {}, {}, entID, 
                    testmode);
            }

            if (!buildObject->outboundTags.isEmpty()) {
                runSpeedTest(QJsonObject2QString(buildObject->coreConfig, false), false, false, 
                buildObject->outboundTags, buildObject->tag2entID, -1, testmode);
            }
        }
        profiles.clear();
        batch = 0;
        skip_loopback:
        if (!stopSpeedtest.load()){
            if ((index < count)){
                goto loop_back;
            }
        }
        
        } else
        {
            stopSpeedtest.store(false);
            // untagged the core would measure route.final, not the profile
            runSpeedTest("", true, true, liveTags, {}, -1,
                testmode);
        }

        runOnUiThread([=,this]{
            refresh_proxy_list();
            MW_show_log(tr("Speedtest finished!"));
        });
    });
}

void MainWindow::querySpeedtest(const QMap<QString, int>& tag2entID, bool testCurrent)
{
    const int testId = live_test_profile_id.load();
    const auto generation = live_test_profile_generation.load();
    if (testCurrent && testId < 0) return;
    bool ok;
    auto res = defaultClient->QueryCurrentSpeedTests(&ok);
    if (!ok || !res->is_running)
    {
        return;
    }
    runOnUiThread([=, this]
    {
        if (testCurrent && (profile_generation.load() != generation || live_test_profile_id.load() != testId || !running || running->id != testId)) return;
        const auto profile = Configs::profileManager->GetProfile(testCurrent ? testId :
            tag2entID[QString::fromUtf8(res->result.outbound_tag.c_str())]);
        if (!profile) return;
        showSpeedtestData = true;
        currentSptProfileName = profile->name;
        auto result = currentTestResult = res->result;
        UpdateDataView();

        if (result.error.empty() && !result.cancelled && 
            QDateTime::currentMSecsSinceEpoch() - last_speedtest_ui_update_ms >= 500)
        {
            auto dl_speed = QString::fromUtf8(result.dl_speed.c_str());
            auto ul_speed = QString::fromUtf8(result.ul_speed.c_str());
            auto latency = result.latency;
            auto country = QString::fromUtf8(result.server_country.c_str());
            if (!dl_speed.isEmpty()) profile->dl_speed = (dl_speed);
            if (!ul_speed.isEmpty()) profile->ul_speed = (ul_speed);
            if (profile->latencyInt <= 0 && latency > 0) profile->latencyInt = latency;
            if (!country.isEmpty()) profile->test_country = CountryNameToCode((country));
            profile->Save();
            refresh_proxy_list(profile->id);
            last_speedtest_ui_update_ms = QDateTime::currentMSecsSinceEpoch();
        }
    });
}

void MainWindow::queryCountryTest(const QMap<QString, int>& tag2entID, bool testCurrent)
{
    const int testId = live_test_profile_id.load();
    const auto generation = live_test_profile_generation.load();
    if (testCurrent && testId < 0) return;
    bool ok;
    auto res = defaultClient->QueryCountryTestResults(&ok);
    if (!ok || res->results.empty())
    {
        return;
    }
    for (const auto& result : res->results)
    {
        runOnUiThread([=, this]
        {
            if (testCurrent && (profile_generation.load() != generation || live_test_profile_id.load() != testId || !running || running->id != testId)) return;
            const auto profile = Configs::profileManager->GetProfile(testCurrent ? testId :
                tag2entID[QString::fromUtf8(result.outbound_tag.c_str())]);
            if (!profile) return;
            if (result.error.empty() && !result.cancelled)
            {
                auto latency = result.latency;
                auto country = QString::fromUtf8(result.server_country.c_str());
                if (profile->latencyInt <= 0 && latency > 0) profile->latencyInt = latency;
                if (!country.isEmpty()) profile->test_country = CountryNameToCode(
                    (country));
                profile->Save();
                refresh_proxy_list(profile->id);
            }
        });
    }
}


void MainWindow::runSpeedTest(const QString& config, bool useDefault, bool testCurrent, 
    const QStringList& outboundTags, const QMap<QString, int>& tag2entID, int entID, const int testmode)
{
    if (stopSpeedtest.load()) {
        MW_show_log(tr("Profile speed test aborted"));
        return;
    }

    libcore::SpeedTestRequest req;
    int speedtestConf = testmode;
    if (speedtestConf < 0) {
        speedtestConf = Configs::dataStore->speed_test_mode;
    }

    req.outbound_tags = QListStr2VectorStr(outboundTags);
    req.config = (config.toStdString());
    req.use_default_outbound = (useDefault);
    req.test_download = (speedtestConf == Configs::TestConfig::FULL || speedtestConf == Configs::TestConfig::DL);
    req.test_upload = (speedtestConf == Configs::TestConfig::FULL || speedtestConf == Configs::TestConfig::UL);
    req.simple_download = ( speedtestConf == Configs::TestConfig::SIMPLEDL);
    req.simple_download_addr = ( Configs::dataStore->simple_dl_url).toStdString();
    req.test_current = ( testCurrent);
    req.timeout_ms = ( Configs::dataStore->speed_test_timeout_ms);
    req.only_country = ( speedtestConf == Configs::TestConfig::COUNTRY);
    req.country_concurrency = ( Configs::dataStore->test_concurrent);

    // loop query result
    const auto doneMu = std::make_shared<std::atomic<bool>>(false);
    const auto pollingDone = std::make_shared<QSemaphore>();
    runOnNewThread([=,this]
    {
        const auto complete = qScopeGuard([pollingDone] { pollingDone->release(); });
        while (true) {
            QThread::msleep(100);
            if (doneMu->load())
            {
                break;
            }
            if (speedtestConf == Configs::TestConfig::COUNTRY)
            {
                queryCountryTest(tag2entID, testCurrent);
            } else
            {
                querySpeedtest(tag2entID, testCurrent);
            }
        }
        runOnUiThread([=, this]
        {
            showSpeedtestData = false;
            UpdateDataView(true);
        });
    });
    bool rpcOK;
    auto result = defaultClient->SpeedTest(&rpcOK, req);
    doneMu->store(true);
    pollingDone->acquire();
    //
    if (!rpcOK || result->results.empty() ) return;

    for (const auto &res: result->results) {
        if (testCurrent) entID = live_test_profile_id.load();
        else {
            auto tag = QString::fromUtf8(res.outbound_tag.c_str());
            entID = tag2entID.count(tag) == 0 ? -1 : tag2entID[tag];
        }
        if (entID == -1) {
            MW_show_log(tr("Something is very wrong, the subject ent cannot be found!"));
            continue;
        }

        auto ent = Configs::profileManager->GetProfile(entID);
        if (ent == nullptr) {
            MW_show_log(tr("Profile manager data is corrupted, try again."));
            continue;
        }

        if (res.cancelled) continue;

        if (res.error.empty()) {
            ent->dl_speed = QString::fromUtf8(res.dl_speed.c_str());
            ent->ul_speed = QString::fromUtf8(res.ul_speed.c_str());
            auto latency = res.latency;
            if (ent->latencyInt <= 0 && latency > 0) ent->latencyInt = latency;
            auto country = res.server_country;
            if (!country.empty()) ent->test_country = 
                CountryNameToCode(country);
        } else {
            ent->dl_speed = "N/A";
            ent->ul_speed = "N/A";
            ent->latencyInt = -1;
            ent->test_country = "";
            MW_show_log(tr("[%1] speed test error: %2").arg(
                ent->DisplayTypeAndName(), QString::fromUtf8(res.error.c_str())));
        }
        ent->Save();
    }
}

bool MainWindow::set_system_dns(bool set, bool save_set) {
    if (!Configs::dataStore->enable_dns_server) {
        MW_show_log(tr("You need to enable hijack DNS server first"));
        return false;
    }
    if (!get_elevated_permissions(4)) {
        return false;
    }
    bool rpcOK;
    QString res;
    if (set) {
        res = defaultClient->SetSystemDNS(&rpcOK, false);
    } else {
        res = defaultClient->SetSystemDNS(&rpcOK, true);
    }
    if (!rpcOK) {
        MW_show_log(tr("Failed to set system dns: ") + res);
        return false;
    }
    if (save_set) Configs::dataStore->system_dns_set = set;
    return true;
}

void MainWindow::profile_start(int id, bool do_not_test) {
    if (QThread::currentThread() != thread()) {
        QMetaObject::invokeMethod(this, [this, id, do_not_test] {
            profile_start(id, do_not_test);
        }, Qt::QueuedConnection);
        return;
    }
    if (Configs::dataStore->prepare_exit) return;
#ifdef Q_OS_UNIX
    if (Configs::dataStore->enable_dns_server && Configs::dataStore->dns_server_listen_port <= 1024 &&
        !get_elevated_permissions()) {
        MW_show_log(QString("Failed to get admin access, cannot listen on port %1 without it")
                        .arg(Configs::dataStore->dns_server_listen_port));
        return;
    }
#endif
    const auto selected = get_now_selected_list();
    if (id < 0 && !selected.isEmpty()) id = selected.first();
    const auto ent = Configs::profileManager->GetProfile(id);
    if (!ent) return;
    const auto group = Configs::profileManager->GetGroup(ent->gid);
    if (!group || group->archive) return;

    profile_operations.enqueue({id, do_not_test, false, false, {}});
    process_profile_operations();
}

namespace {
struct ProfileStopResult {
    bool ok = false;
    QString error;
    QList<int> savedIds;
};
struct ProfileStartResult {
    bool ok = false;
    QString error;
};

void showProfileError(QWidget *parent, const QString &title, const QString &error,
                      std::function<void()> reset = {}) {
    // Do not hold the operation queue inside a modal exec() event loop.
    auto message = new QMessageBox(QMessageBox::Warning, title, error, QMessageBox::Ok, parent);
    message->setAttribute(Qt::WA_DeleteOnClose);
    if (reset) {
        const auto button = message->addButton(MainWindow::tr("Reset"), QMessageBox::ActionRole);
        QObject::connect(button, &QPushButton::clicked, message, [message, reset] {
            message->close();
            reset();
        });
    }
    message->open();
}
}

void MainWindow::process_profile_operations() {
    const auto next = profile_operations.next();
    if (!next) return;
    const auto operation = *next;
    const auto finish = [this, operation] {
        profile_operations.finish();
        if (operation.completed) operation.completed();
        QTimer::singleShot(0, this, &MainWindow::process_profile_operations);
    };

    const auto start = [this, operation, finish] {
        if (operation.id < 0 || Configs::dataStore->prepare_exit) {
            finish();
            return;
        }
        const auto ent = Configs::profileManager->GetProfile(operation.id);
        const auto group = ent ? Configs::profileManager->GetGroup(ent->gid) : nullptr;
        if (!ent || !group || group->archive) {
            finish();
            return;
        }
        if (!Configs::dataStore->core_running) {
            runOnThread([this, id = ent->id] {
                core_process->start_profile_when_core_is_up = id;
            }, DS_cores);
            finish();
            return;
        }

        MW_show_log(">>>>>>>> " + tr("Starting profile %1").arg(ent->DisplayTypeAndName()));
        ui->start_stop_button->setState(Icon::State::CONNECTING);
        auto watcher = new QFutureWatcher<ProfileStartResult>(this);
        connect(watcher, &QFutureWatcher<ProfileStartResult>::finished, this,
                [this, watcher, ent, operation, finish] {
            const auto result = watcher->result();
            watcher->deleteLater();
            if (result.ok) {
                ++profile_generation;
                running = ent;
                Configs::dataStore->UpdateStartedId(ent->id);
                Stats::trafficLooper->setEnabled(true);
                Stats::connection_lister->setSuspended(false);
            } else {
                MW_show_log("<<<<<<<< " + tr("Failed to start profile %1").arg(ent->DisplayTypeAndName()));
                if (!result.error.isEmpty() && !Configs::dataStore->prepare_exit)
                    showProfileError(this, tr("LoadConfig return error"), result.error,
                        result.error.contains("configure tun interface") ?
                            std::function<void()>([this] { StopVPNProcess(); }) : std::function<void()>{});
            }
            ui->start_stop_button->setState(result.ok ? Icon::State::RUNNING : Icon::State::IDLE);
            refresh_status();
            refresh_proxy_list(ent->id);
            // Do not launch a latency test for an instance already queued to stop.
            const bool test = result.ok && !operation.doNotTest && !profile_operations.hasPending() &&
                              !Configs::dataStore->prepare_exit;
            finish();
            if (test) QTimer::singleShot(0, this, [this] {
                if (running && !profile_operations.hasPending()) url_test_current();
            });
        });
        watcher->setFuture(QtConcurrent::run([ent] {
            ProfileStartResult out;
            try {
                bool rpcOK = false;
                auto [error, config] = API::defaultClient->StartEntity(&rpcOK, ent);
                out.error = error;
                out.ok = rpcOK && error.isEmpty() && config != nullptr;
                if (out.ok) {
                    QMutexLocker lock(&Stats::trafficLooper->loop_mutex);
                    Stats::trafficLooper->items = config->outboundStats;
                    Stats::trafficLooper->isChain = ent->type == "chain";
                }
            } catch (const std::exception &e) {
                out.error = QString::fromUtf8(e.what());
            } catch (...) {
                out.error = QStringLiteral("Unexpected error while starting the profile");
            }
            return out;
        }));
    };

    if (!running) {
        start();
        return;
    }
    const auto oldProfile = running;
    const auto id = oldProfile->id;
    ++profile_generation;
    live_test_profile_id = -1;
    Stats::trafficLooper->setEnabled(false);
    Stats::connection_lister->setSuspended(true);
    UpdateConnectionListWithRecreate({});
    ui->start_stop_button->setState(Icon::State::DISCONNECTING);
    MW_show_log(">>>>>>>> " + tr("Stopping profile %1").arg(oldProfile->DisplayTypeAndName()));

    auto watcher = new QFutureWatcher<ProfileStopResult>(this);
    connect(watcher, &QFutureWatcher<ProfileStopResult>::finished, this,
            [this, watcher, operation, id, start, finish] {
        const auto result = watcher->result();
        watcher->deleteLater();
        for (const int savedId : result.savedIds) refresh_proxy_list(savedId);
        if (!result.ok) {
            // Keep ownership in the GUI too. An RPC timeout is NOT a successful
            // stop, and launching another TUN now can deadlock both instances.
            MW_show_log("<<<<<<<< " + tr("Failed to stop, please restart the program."));
            ui->start_stop_button->setState(Icon::State::RUNNING);
            if (!result.error.isEmpty() && !Configs::dataStore->prepare_exit)
                showProfileError(this, tr("Stop return error"), result.error,
                                 [this] { StopVPNProcess(); });
            finish();
            return;
        }
        running = nullptr;
        ui->start_stop_button->setState(Icon::State::IDLE);
        if (operation.manual) Configs::dataStore->UpdateStartedId(-1919);
        Configs::dataStore->need_keep_vpn_off = false;
        refresh_status();
        refresh_proxy_list_impl_refresh_data(id, true);
        start();
    });
    watcher->setFuture(QtConcurrent::run([operation] {
        ProfileStopResult out;
        try {
            // Both the lock and the last RPC flush belong on a worker. Waiting
            // on this mutex in the GUI used to freeze every settings restart.
            {
                QMutexLocker lock(&Stats::trafficLooper->loop_mutex);
                if (!operation.crash) Stats::trafficLooper->UpdateAll();
                for (const auto &item : Stats::trafficLooper->items) {
                    if (item->id < 0) continue;
                    const auto profile = Configs::profileManager->GetProfile(item->id);
                    if (!profile) continue;
                    profile->Save();
                    out.savedIds.append(item->id);
                }
            }
            if (operation.crash) {
                out.ok = true;
            } else {
                bool rpcOK = false;
                out.error = API::defaultClient->Stop(&rpcOK);
                out.ok = rpcOK && out.error.isEmpty();
            }
        } catch (const std::exception &e) {
            out.error = QString::fromUtf8(e.what());
        } catch (...) {
            out.error = QStringLiteral("Unexpected error while stopping the profile");
        }
        return out;
    }));
}

bool MainWindow::set_spmode_system_proxy(bool enable, bool save) {
    #ifndef USE_CPP_PROXY_CONFIGURATOR
    bool isok = true;
    int inbound_proxy_type = Configs::dataStore->inbound_proxy_type->value;
    bool socks_supported = inbound_proxy_type == 2;
    if (!socks_supported){
        if (inbound_proxy_type != 1){
            enable = false;
        }
    }
    #endif
    if (enable != Configs::dataStore->spmode_system_proxy) {
        if (enable) {
            auto socks_port = Configs::dataStore->inbound_socks_port;
            #ifdef USE_CPP_PROXY_CONFIGURATOR
            SetSystemProxy(socks_port, socks_port, Configs::dataStore->proxy_scheme);
            #else
            defaultClient->EnableSystemProxy(Configs::dataStore->inbound_address, 
                socks_port, socks_supported, &isok);
            #endif
        } else {
            #ifdef USE_CPP_PROXY_CONFIGURATOR
            ClearSystemProxy();
            #else
            defaultClient->DisableSystemProxy(&isok);
            #endif
        }
    }
    #ifndef USE_CPP_PROXY_CONFIGURATOR
    #ifdef DEBUG_MODE
    qDebug() << "System proxy set to " << enable << " with status " << isok;
    #endif 
    if (!isok){
        enable = !enable;
    }
    #endif

    if (save) {
        Configs::dataStore->remember_spmode.removeAll("system_proxy");
        if (enable && Configs::windowSettings->remember_last_profile) {
            Configs::dataStore->remember_spmode.append("system_proxy");
        }
        Configs::dataStore->Save();
    }

    Configs::dataStore->spmode_system_proxy = enable;
    refresh_status();
    return enable;
}

void MainWindow::profile_stop(bool crash, bool block, bool manual) {
    if (QThread::currentThread() != thread()) {
        // Core callbacks and shutdown can originate on worker threads. Widgets,
        // running and the operation queue must only be touched by the GUI.
        const auto done = std::make_shared<QSemaphore>();
        QMetaObject::invokeMethod(this, [this, crash, manual, done] {
            profile_operations.enqueue({-1, true, crash, manual, [done] { done->release(); }});
            process_profile_operations();
        }, Qt::QueuedConnection);
        if (block) done->acquire();
        return;
    }

    const auto done = std::make_shared<QSemaphore>();
    profile_operations.enqueue({-1, true, crash, manual, [done] { done->release(); }});
    process_profile_operations();
    if (block) {
        // Only legacy shutdown/suspend callers need a synchronous completion.
        // Normal switching never nests an event loop or waits on a GUI mutex.
        while (!done->tryAcquire(1, 20))
            QCoreApplication::processEvents(QEventLoop::ExcludeUserInputEvents, 20);
    }
}
