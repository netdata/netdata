#ifndef REPLICATION_LOGGER_H
#define REPLICATION_LOGGER_H

#include "replication-private.h"
#include <iomanip>
#include <ctime>
#include "ml/json/single_include/nlohmann/json.hpp"

namespace replication {

using JSON = nlohmann::json;

static void formatTR(JSON &J, const TimeRange &TR, const char *Fmt) {
    const size_t Len = 1024;

    char AfterBuf[Len];
    time_t After = TR.first;
    auto AfterTM = *std::localtime(&After);
    strftime(AfterBuf, Len, Fmt, &AfterTM);

    char BeforeBuf[Len];
    time_t Before = TR.second;
    auto BeforeTM = *std::localtime(&Before);
    strftime(BeforeBuf, Len, Fmt, &BeforeTM);

    time_t Duration = Before - After + 1;

    char OutBuf[Len];
    snprintfz(OutBuf, Len, "[%s, %s] (Duration=%ld)", AfterBuf, BeforeBuf, Duration);

    if (J.is_array())
        J.push_back(OutBuf);
    else
        J["TR"] = OutBuf;
}

class Logger {
public:
    Logger(const char *Hostname) : Hostname(Hostname) {}

    void createdHost(std::vector<TimeRange> &TRs) {
        if (!Cfg.EnableLogging)
            return;

        nlohmann::json J;

        J["new-host"]["name"] = Hostname;
        J["new-host"]["gaps"] = nlohmann::json::array();

        auto &GapsArray = J["new-host"]["gaps"];
        for (const TimeRange &TR : TRs) {
            nlohmann::json JO;
            formatTR(JO, TR, "%d/%m/%Y - %H:%M:%S");
            GapsArray.push_back(JO);
        }

        log(J);
    }

    void deletedHost(std::vector<TimeRange> &TRs) {
        if (!Cfg.EnableLogging)
            return;

        nlohmann::json J;

        J["delete-host"]["name"] = Hostname;
        J["delete-host"]["gaps"] = nlohmann::json::array();

        auto &GapsArray = J["delete-host"]["gaps"];
        for (const TimeRange &TR : TRs) {
            nlohmann::json JO;
            formatTR(JO, TR, "%d/%m/%Y - %H:%M:%S");
            GapsArray.push_back(JO);
        }

        log(J);
    }

    void startedReplicationThread() {
        if (!Cfg.EnableLogging)
            return;

        nlohmann::json J;
        J["started-replication-thread"] = Hostname;
        log(J);
    }

    void stoppedReplicationThread() {
        if (!Cfg.EnableLogging)
            return;

        nlohmann::json J;
        J["stopped-replication-thread"] = Hostname;
        log(J);
    }

    void receiverSentGaps(std::vector<TimeRange> &TRs) {
        if (!Cfg.EnableLogging)
            return;

        nlohmann::json J;

        J["receiver-sent-gaps"]["host"] = Hostname;
        J["receiver-sent-gaps"]["gaps"] = nlohmann::json::array();

        auto &GapsArray = J["receiver-sent-gaps"]["gaps"];
        for (const TimeRange &TR : TRs) {
            nlohmann::json JO;
            formatTR(JO, TR, "%d/%m/%Y - %H:%M:%S");
            GapsArray.push_back(JO);
        }
        log(J);
    }

    void receiverFilledGap(const GapData &GD) {
        if (!Cfg.EnableLogging)
            return;

        std::stringstream SS;
        SS << Hostname << "." << GD.getChart() << "." << GD.getDimension();

        const auto &SpanInfo = GD.getTimeRangeSpan();
        if (SpanInfo.first == 0)
            return;

        TimeRange TR = SpanInfo.second;

        {
            std::lock_guard<std::mutex> L(Mutex);
            ReceiverFilledGapsMap[SS.str()].push_back(TR);
        }
    }

    void receiverDroppedGap(const TimeRange &TR) {
        if (!Cfg.EnableLogging)
            return;

        nlohmann::json J;
        J["receiver-dropped-gap"] = { { "host", Hostname } };
        formatTR(J["receiver-dropped-gap"], TR, "%d/%m/%Y - %H:%M:%S");
        log(J);
    }

    void senderReceivedGaps(std::vector<TimeRange> &TRs) {
        if (!Cfg.EnableLogging)
            return;

        nlohmann::json J;

        J["sender-received-gaps"]["host"] = Hostname;
        J["sender-received-gaps"]["gaps"] = nlohmann::json::array();

        auto &GapsArray = J["sender-received-gaps"]["gaps"];
        for (const TimeRange &TR : TRs) {
            nlohmann::json JO;
            formatTR(JO, TR, "%d/%m/%Y - %H:%M:%S");
            GapsArray.push_back(JO);
        }

        log(J);
    }

    void senderFilledGap(const GapData &GD) {
        if (!Cfg.EnableLogging)
            return;

        std::stringstream SS;
        SS << Hostname << "." << GD.getChart() << "." << GD.getDimension();

        const auto &SpanInfo = GD.getTimeRangeSpan();
        if (SpanInfo.first == 0)
            return;

        TimeRange TR = SpanInfo.second;

        {
            std::lock_guard<std::mutex> L(Mutex);
            SenderFilledGapsMap[SS.str()].push_back(TR);
        }
    }

    void senderDroppedGap(const TimeRange &TR) {
        if (!Cfg.EnableLogging)
            return;

        nlohmann::json J;
        J["sender-dropped-gap"] = { { "host", Hostname } };
        formatTR(J["sender-dropped-gap"], TR, "%d/%m/%Y - %H:%M:%S");
        log(J);
    }

    std::string serialize() {
        std::lock_guard<std::mutex> L(Mutex);

        JSON JRes = JD;

        /*
         * Add any filled gaps on the receiver side
         */
        if (ReceiverFilledGapsMap.size()) {
            JRes.push_back(nlohmann::json::object());
            auto &J = JRes.back();

            J["receiver-filled-gaps"] = nlohmann::json::array();
            auto &GapsArray = J["receiver-filled-gaps"];
            for (const auto &P : ReceiverFilledGapsMap) {
                nlohmann::json J;

                const std::string &Id = P.first;
                const std::vector<TimeRange> &TRs = P.second;

                J[Id] = nlohmann::json::array();
                for (const TimeRange &TR : TRs)
                    formatTR(J[Id], TR, "%H:%M:%S");

                GapsArray.push_back(J);
            }
        }

        /*
         * Rinse & repeat for the sender side
         */
        if (SenderFilledGapsMap.size()) {
            JRes.push_back(nlohmann::json::object());
            auto &J = JRes.back();

            J["sender-filled-gaps"] = nlohmann::json::array();
            auto &GapsArray = J["sender-filled-gaps"];
            for (const auto &P : SenderFilledGapsMap) {
                nlohmann::json J;

                const std::string &Id = P.first;
                const std::vector<TimeRange> &TRs = P.second;

                J[Id] = nlohmann::json::array();
                for (const TimeRange &TR : TRs)
                    formatTR(J[Id], TR, "%H:%M:%S");

                GapsArray.push_back(J);
            }
        }

        return JRes.dump(/* indent */ 4);
    }

private:
    void log(JSON &J, bool addTime=true) {
        std::lock_guard<std::mutex> L(Mutex);

        if (addTime) {
            char Buf[1024];
            time_t NowT = now_realtime_sec();
            auto NowTM = *std::localtime(&NowT);
            strftime(Buf, 1024, "%d/%m/%Y - %H:%M:%S", &NowTM);
            J["date"] = Buf;
        }

        JD.push_back(J);
    }

private:
    const char *Hostname;
    JSON JD;
    std::mutex Mutex;

    std::map<std::string, std::vector<TimeRange>> ReceiverFilledGapsMap;
    std::map<std::string, std::vector<TimeRange>> SenderFilledGapsMap;
};

} // namespace replication

#endif /* REPLICATION_LOGGER_H */
