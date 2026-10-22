

#include <nekobox/api/RPC.h>
#include <nekobox/api/ResponsiveTask.hpp>
#include <nekobox/dataStore/Configs.hpp>
#include <QDebug>
#include <optional>
#include <qnamespace.h>

//Thrift libraries
#include <thrift/protocol/TBinaryProtocol.h>             
#include <thrift/server/TSimpleServer.h>
#include <thrift/transport/TSocket.h>                    
#include <thrift/transport/TPipe.h>                    
#include <thrift/transport/TBufferTransports.h>          
#include <thrift/transport/TTransportUtils.h>
#include <gen-cpp/LibcoreService.h>
#include <gen-cpp/libcore_types.h>
#include <QString>

using namespace apache::thrift;
using namespace apache::thrift::protocol;
using namespace apache::thrift::transport;

struct Status{
    bool ok = false;
    std::string what = "";
    bool isOk(){ return ok; }
    QString message() { return QString::fromUtf8(what.c_str()); }
};

namespace API {

    void Client::onError(const QString & errStr){
        MW_show_log("[Error] Core: " + errStr);
    }

    Client::Client() {
    }

#define CHECK(method) \
bool is_running = true;                                  \
if (!Configs::dataStore->core_running) {                                               \
    onError("Cannot invoke method " + QString(method) + ", core is not running");  \
    is_running = false; \
} else {    \
}

// Without these a call blocks forever when the core stops answering, and since
// some calls are made from the GUI thread that shows up as a hung window.
// The read budget has to outlast the slowest handler (url and speed tests),
// the core itself gives up on a socket after 10s.
constexpr int kRpcConnectTimeoutMs = 2000;
constexpr int kRpcSendTimeoutMs = 5000;
constexpr int kRpcRecvTimeoutMs = 20000;

static std::shared_ptr<TTransport> getThriftTransport(const char *method){
    int port = Configs::dataStore->core_port;
    std::shared_ptr<TSocket> socket;
    if (port > 0){
        socket = std::make_shared<TSocket>(Configs::dataStore->core_domain, port);
    } else {
        socket = std::make_shared<TSocket>(Configs::dataStore->core_domain);
    }
    socket->setConnTimeout(kRpcConnectTimeoutMs);
    socket->setSendTimeout(kRpcSendTimeoutMs);
    const QByteArray name(method);
    // A stale statistics poll must not hold the traffic lock for 20 seconds
    // while a profile switch is waiting to flush its counters.
    const bool polling = name == "QueryStats" || name == "ListConnections" ||
                         name == "QueryURLTest" || name == "QueryIPTest" ||
                         name == "QuerySpeedTest" || name == "QueryCountryTest" || name == "StopTest";
    socket->setRecvTimeout(polling ? 3000 : (name == "Stop" ? 10000 : kRpcRecvTimeoutMs));
    return socket;
}

template <typename Response> struct RpcReply {
    Status status;
    std::optional<Response> reply;
};

#define CHANNEL(X, VAL) \
auto socketAA = getThriftTransport(#X); \
auto rpcReplyAA = responsiveTask([socketAA, request] { \
    RpcReply<libcore::VAL> resultAA; \
    try { \
        auto transportAA = std::make_shared<TBufferedTransport>(socketAA); \
        auto protocolAA = std::make_shared<TBinaryProtocol>(transportAA); \
        libcore::LibcoreServiceClient clientAA(protocolAA); \
        transportAA->open(); \
        libcore::VAL responseAA; \
        clientAA.X(responseAA, request); \
        transportAA->close(); \
        resultAA.reply = std::move(responseAA); \
        resultAA.status.ok = true; \
    } catch (const std::exception &e) { \
        resultAA.status.what = e.what(); \
    } \
    return resultAA; \
}); \
auto status = std::move(rpcReplyAA.status); \
auto reply = std::move(rpcReplyAA.reply);

#define NOT_OK                                                      \
    *rpcOK = false;                                                 \
    onError(QString("LibcoreService error: %1\n").     \
    arg(status.message())                          \
);

    QString Client::Start(bool *rpcOK, const libcore::LoadConfigReq &request) {
        CHECK("Start")
        if (!is_running){
            *rpcOK = false;
            return "";
        }

        CHANNEL(Start, ErrorResp)

        if(status.isOk()) {
            *rpcOK = true;
            return QString::fromUtf8(reply->error.c_str());
        } else {
            NOT_OK
            return  status.message();
        }
    }

    QString Client::Stop(bool *rpcOK) {
        CHECK("Stop")
        if (!is_running){
            *rpcOK = false;
            return "";
        }
        libcore::EmptyReq request;
        CHANNEL(Stop, ErrorResp)
        
        if(status.isOk()) {
            *rpcOK = true;
            return QString::fromUtf8(reply->error.c_str());
        } else {
            NOT_OK
            return status.message();
        }
    }

    QString Client::EnableSystemProxy(const QString& address, int port, bool isSocksSupported, bool *rpcOK){
        CHECK("EnableSystemProxy")
        if (!is_running){
            *rpcOK = false;
            return "";
        }
        libcore::SystemProxy request;
        request.port = port;
        request.support_socks = isSocksSupported;
        request.address = address.toStdString();
        CHANNEL(EnableSystemProxy, ErrorResp)
        if(status.isOk()) {
            *rpcOK = true;
            return QString::fromUtf8(reply->error.c_str());
        } else {
            NOT_OK
            return status.message();
        }
    };
    QString Client::DisableSystemProxy(bool *rpcOK){
        CHECK("DisableSystemProxy")
        if (!is_running){
            *rpcOK = false;
            return "";
        }
        libcore::EmptyReq request;
        CHANNEL(DisableSystemProxy, ErrorResp)
        if(status.isOk()) {
            *rpcOK = true;
            return QString::fromUtf8(reply->error.c_str());
        } else {
            NOT_OK
            return status.message();
        }
    };

    std::optional<libcore::QueryStatsResp> Client::QueryStats() {
        CHECK("QueryStats")
        if (!is_running){
            return std::nullopt;
        }
        libcore::EmptyReq request;
        CHANNEL(QueryStats, QueryStatsResp)

        if(status.isOk()) {
            return reply;
        } else {
            return std::nullopt;
        }
    }

    std::optional<libcore::CacheURLResult> Client::CacheHTTP(bool * rpcOK, const libcore::CacheURLRequest &request){
        CHECK("CacheHTTP")
        if (!is_running){
            *rpcOK = false;
            return std::nullopt;
        }
        CHANNEL(CacheHTTP, CacheURLResult)

        if(status.isOk()) {
            *rpcOK = true;
            return reply;
        } else {
            NOT_OK
            return std::nullopt;
        }
    }

    std::optional<libcore::TestResp> Client::Test(bool *rpcOK, const libcore::TestReq &request) {
        CHECK("Test")
        if (!is_running){
            *rpcOK = false;
            return std::nullopt;
        }
        CHANNEL(Test, TestResp)
    
        if(status.isOk()) {
            *rpcOK = true;
            return reply;
        } else {
            NOT_OK
            return std::nullopt;
        }
    }

    void Client::StopTests(bool *rpcOK) {
        CHECK("StopTests")
        
        if (!is_running){
            *rpcOK = false;
            return;
        }
        libcore::EmptyReq request;
        CHANNEL(StopTest, EmptyResp)

        if(status.isOk()) {
            *rpcOK = true;
            return;
        } else {
            NOT_OK
            return;
        }
    }

    std::optional<libcore::QueryURLTestResponse> Client::QueryURLTest(bool *rpcOK)
    {
        CHECK("QueryURLTest")
        if (!is_running){
            *rpcOK = false;
            return std::nullopt;
        }
        libcore::EmptyReq request;
        CHANNEL(QueryURLTest, QueryURLTestResponse)
     
        if(status.isOk()) {
            *rpcOK = true;
            return reply;
        } else {
            NOT_OK
            return std::nullopt;
        }
    }

    std::optional<libcore::QueryIPTestResponse> Client::QueryIPTest(bool *rpcOK)
    {
        CHECK("QueryIPTest")
        if (!is_running){
            *rpcOK = false;
            return std::nullopt;
        }
        libcore::EmptyReq request;
        CHANNEL(QueryIPTest, QueryIPTestResponse)
     
        if(status.isOk()) {
            *rpcOK = true;
            return reply;
        } else {
            NOT_OK
            return std::nullopt;
        }
    }


    std::optional<libcore::GenWgKeyPairResponse> Client::GenWgKeyPair(bool *rpcOK)
    {
        CHECK("GenWgKeyPair")
        if (!is_running){
            *rpcOK = false;
            return std::nullopt;
        }
        libcore::EmptyReq request;
        CHANNEL(GenWgKeyPair, GenWgKeyPairResponse)
     
        if(status.isOk()) {
            *rpcOK = true;
            return reply;
        } else {
            NOT_OK
            return std::nullopt;
        }
    }

    QString Client::SetSystemDNS(bool *rpcOK, const bool clear) const {
        CHECK("SetSystemDNS")
        if (!is_running){
            *rpcOK = false;
            return "";
        }
        
        libcore::SetSystemDNSRequest request;
        request.clear = (clear);
        libcore::EmptyResp resp;
        CHANNEL(SetSystemDNS, EmptyResp)

        if(status.isOk()) {
            *rpcOK = true;
            return "";
        } else {
            NOT_OK
            return (status.message());
        }
    }

    std::optional<libcore::ListConnectionsResp> Client::ListConnections(bool* rpcOK) const
    {
        CHECK("ListConnections")
        if (!is_running){
            *rpcOK = false;
            return std::nullopt;
        }
    
        libcore::EmptyReq request;
        CHANNEL(ListConnections, ListConnectionsResp)
        if(status.isOk()) {
            *rpcOK = true;
            return reply;
        } else {
            // Listing connections fails whenever the core is busy; logging
            // that every second floods the UI and can freeze the window.
            *rpcOK = false;
            return std::nullopt;
        }
    }

    QString Client::CheckConfig(bool* rpcOK, const QString& config) const
    {
        CHECK("CheckConfig")
        if (!is_running){
            *rpcOK = false;
            return "";
        }
        libcore::LoadConfigReq request;
        request.core_config = (config.toStdString());
        CHANNEL(CheckConfig, ErrorResp)
        if(status.isOk())
        {
            *rpcOK = true;
            return QString::fromUtf8(reply->error.c_str());
        } else
        {
            NOT_OK
            return (status.message());
        }
    }

    bool Client::IsPrivileged(bool* rpcOK) const
    {
        CHECK("IsPrivileged")
        if (!is_running) {
            *rpcOK = false;
            return false;
        }

        libcore::EmptyReq request;
        request.start = true;
        CHANNEL(IsPrivileged, IsPrivilegedResponse)
        if(status.isOk()) {
            *rpcOK = true;
            return reply->has_privilege;
        } else {
            NOT_OK
            return false;
        }
    }

    std::optional<libcore::SpeedTestResponse> Client::SpeedTest(bool *rpcOK, const libcore::SpeedTestRequest &request)
    {
        CHECK("SpeedTest")
        if (!is_running){
            *rpcOK = false;
            return std::nullopt;
        }
        CHANNEL(SpeedTest, SpeedTestResponse)

        if(status.isOk()) {
            *rpcOK = true;
            return reply;
        } else {
            NOT_OK
            return reply;
        }
    }

    std::optional<libcore::QuerySpeedTestResponse> Client::QueryCurrentSpeedTests(bool *rpcOK)
    {
        CHECK("QueryCurrentSpeedTests")
        if (!is_running){
            *rpcOK = false;
            return std::nullopt;
        }
        libcore::EmptyReq request;
        CHANNEL(QuerySpeedTest, QuerySpeedTestResponse)

        if(status.isOk()) {
            *rpcOK = true;
            return reply;
        } else {
            NOT_OK
            return std::nullopt;
        }
    }

    std::optional<libcore::QueryCountryTestResponse> Client::QueryCountryTestResults(bool* rpcOK)
    {
        CHECK("QueryCountryTestResults")
        if (!is_running){
            *rpcOK = false;
            return std::nullopt;
        }
        const libcore::EmptyReq request;
        CHANNEL(QueryCountryTest, QueryCountryTestResponse)

        if(status.isOk()) {
            *rpcOK = true;
            return reply;
        } else {
            NOT_OK
            return reply;
        }
    }


    std::tuple<QString, std::shared_ptr<Configs::BuildConfigResult>> Client::StartEntity(bool *rpcOK,
                            const std::shared_ptr<Configs::ProxyEntity> ent) {
        std::shared_ptr<Configs::BuildConfigResult> result = Configs::BuildConfig(ent, false, false);

        if (!result->error.isEmpty()) {
            *rpcOK = false;
            return std::make_tuple(result->error, result);
        }

        libcore::LoadConfigReq req;
        req.core_config =
        (QJsonObject2QString(result->coreConfig, true)).toStdString();
        req.disable_stats = (Configs::dataStore->disable_traffic_stats);
        if (ent->type == "extracore") {
            req.need_extra_process = (true);
            req.extra_process_path = (result->extraCoreData->path).toStdString();
            req.extra_process_args = (result->extraCoreData->args).toStdString();
            req.extra_process_conf = (result->extraCoreData->config).toStdString();
            req.extra_process_conf_dir = (result->extraCoreData->configDir).toStdString();
            req.extra_no_out = (result->extraCoreData->noLog);
        }
        return std::make_tuple(Start(rpcOK, req), result);
    };

} // namespace API
