#include <nekobox/global/HTTPRequestHelperGui.hpp>
#include <nekobox/dataStore/Configs.hpp>
#include <nekobox/ui/mainwindow.h>
#include <QDir>
#include <QFileInfo>
#include <QPointer>
#include <QSaveFile>
#include <algorithm>

QString Configs_network::NetworkRequestHelperGui::DownloadAsset(const QString &url, const QString &fileName)
{
    QString result;
    QEventLoop loop;
    const QString filePath = QDir(Configs::GetBasePath()).absoluteFilePath(fileName);
    const QPointer<MainWindow> window(GetMainWindow());
    const int attempts = std::clamp(Configs::dataStore->download_retries, 1, 5);
    auto future = QtConcurrent::run([=, &result, &loop] {
        QDir().mkpath(QFileInfo(filePath).absolutePath());
        result = "Download failed";
        for (int attempt = 0; attempt < attempts; ++attempt) {
            // Do not append to unverified bytes: a server can ignore Range or
            // an asset can be replaced between download attempts.
            QSaveFile file(filePath);
            file.setDirectWriteFallback(false);
            if (!file.open(QIODevice::WriteOnly)) {
                result = file.errorString();
                break;
            }
            cpr::Session session;
            BuildSession(url, false, session);
            session.SetVerifySsl(cpr::VerifySsl{true});
            qint64 received = 0;
            constexpr qint64 maxAssetSize = 1024LL * 1024 * 1024;
            session.SetWriteCallback(cpr::WriteCallback{[&](std::string_view data, intptr_t) {
                if (data.size() > static_cast<size_t>(maxAssetSize - received)) return false;
                const auto size = static_cast<qint64>(data.size());
                if (file.write(data.data(), size) != size) return false;
                received += size;
                return true;
            }});
            session.SetProgressCallback(cpr::ProgressCallback{[=](cpr::cpr_pf_arg_t total, cpr::cpr_pf_arg_t now,
                                            cpr::cpr_pf_arg_t, cpr::cpr_pf_arg_t, intptr_t) {
                if (total > maxAssetSize) return false;
                runOnUiThread([=] {
                    if (!window || window->isShowRuleSetData()) return;
                    window->setDownloadReport(DownloadProgressReport{fileName, (qint64)now, (qint64)total}, true);
                    window->UpdateDataView();
                });
                return true;
            }});
            const auto response = session.Get();
            if (!response.error && response.status_code == 200 && received > 0) {
                if (file.commit()) result.clear();
                else result = file.errorString();
                break;
            }
            file.cancelWriting();
            result = response.error ? QString::fromStdString(response.error.message)
                                    : QString("HTTP %1 or empty download").arg(response.status_code);
            if (attempt + 1 < attempts) QThread::msleep(500 * (attempt + 1));
        }
        runOnUiThread([=] {
            if (!window) return;
            window->setDownloadReport({}, false);
            window->UpdateDataView(true);
        });
        QMetaObject::invokeMethod(&loop, "quit", Qt::QueuedConnection);
    });
    loop.exec();
    future.waitForFinished();
    return result;
}
