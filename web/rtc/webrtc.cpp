// SPDX-License-Identifier: GPL-3.0-or-later

#include "webrtc.h"

#include "rtc/rtc.hpp"
#include <chrono>
#include <memory>
#include <thread>

using namespace std::chrono_literals;
using std::shared_ptr;
using std::weak_ptr;
template <class T> weak_ptr<T> make_weak_ptr(shared_ptr<T> ptr) { return ptr; }

void webrtc_initialize() {
    rtc::InitLogger(rtc::LogLevel::Warning);
}

class webrtc_connection {
    public:
        size_t version;
        rtc::Configuration config;
        shared_ptr<rtc::PeerConnection> pc;
        rtc::PeerConnection::State state;
        rtc::PeerConnection::GatheringState gstate;

        struct {
            SPINLOCK spinlock;
            size_t active;
            size_t data_channel_id;
            shared_ptr<rtc::DataChannel> dc[DATACHANNEL_ENTRIES_MAX];
        } unsafe;

        struct webrtc_answer answer;
};

void *webrtc_answer_to_offer(const char *sdp) {
    auto *rc = (webrtc_connection *)callocz(1, sizeof(webrtc_connection));
    netdata_spinlock_init(&rc->unsafe.spinlock);

    // config.iceServers.emplace_back("stun.l.google.com:19302");

    rc->pc = std::make_shared<rtc::PeerConnection>(rc->config);

    rc->pc->onLocalDescription([&](rtc::Description description) {
        internal_error(true, "WEBRTC: DESCRIPTION %zu: %s", rc->answer.description_id, std::string(description).c_str());

        if (rc->answer.description_id >= DESCRIPTION_ENTRIES_MAX)
            internal_fatal(true, "WEBRTC: max descriptions size reached.");

        rc->answer.description[rc->answer.description_id++] = strdupz(std::string(description).c_str());
    });

    rc->pc->onLocalCandidate([&](rtc::Candidate candidate) {
        internal_error(true, "WEBRTC: CANDIDATE %zu: %s", rc->answer.candidates_id, std::string(candidate).c_str());

        if (rc->answer.description_id >= CANDIDATES_ENTRIES_MAX)
            internal_fatal(true, "WEBRTC: max candidates size reached.");

        rc->answer.candidates[rc->answer.candidates_id++] = strdupz(std::string(candidate).c_str());
    });

    rc->pc->onStateChange([&](rtc::PeerConnection::State state) {
        internal_error(true, "WEBRTC: STATE: %d", (int)state);
        rc->version++;
        rc->state = state;
    });
    rc->pc->onGatheringStateChange([&](rtc::PeerConnection::GatheringState state) {
        internal_error(true, "WEBRTC: GATHERING STATE: %d", (int)state);
        rc->version++;
        rc->gstate = state;
    });

    rc->pc->onDataChannel([&](shared_ptr<rtc::DataChannel> _dc) {
        internal_error(true, "WEBRTC: DATA CHANNEL '%s' OPEN", _dc->label().c_str());

        netdata_spinlock_lock(&rc->unsafe.spinlock);
        rc->unsafe.active++;
        bool found = false;
        for(size_t i = 0; i < rc->unsafe.data_channel_id ; i++)
            if(!rc->unsafe.dc[i]) {
                rc->unsafe.dc[rc->unsafe.data_channel_id++] = _dc;
                found = true;
            }

        if(!found) {
            if (rc->answer.description_id >= DATACHANNEL_ENTRIES_MAX)
                internal_fatal(true, "WEBRTC: max data channels size reached.");

            rc->unsafe.dc[rc->unsafe.data_channel_id++] = _dc;
        }
        netdata_spinlock_unlock(&rc->unsafe.spinlock);

        _dc->onClosed(
                [&]() {
                    internal_error(true, "WEBRTC: DATA CHANNEL '%s' CLOSED", _dc->label().c_str());
                    netdata_spinlock_lock(&rc->unsafe.spinlock);
                    rc->unsafe.active--;
                    for(size_t i = 0; i < rc->unsafe.data_channel_id ; i++)
                        if(rc->unsafe.dc[i] == _dc)
                            rc->unsafe.dc[i] = nullptr;

                    netdata_spinlock_unlock(&rc->unsafe.spinlock);
                });

        _dc->onMessage([&](auto data) {
            if (std::holds_alternative<std::string>(data)) {
                internal_error(true, "WEBRTC: DATA CHANNEL '%s' MSG: %s", _dc->label().c_str(),
                               std::get<std::string>(data).c_str());
            }
        });
    });

    const char *s = sdp, *e;
    std::string descr, candidates;
    while(*s) {
        if(*s == '\r' || *s == '\n') {
            s++;
            continue;
        }

        e = s;
        while(*e && *e != '\r' && *e != '\n')
            e++;

        if(strncmp(s, "a=candidate:", 12) == 0) {
            if(!candidates.empty())
                candidates.append("\r\n");

            candidates.append(s, e - s);
        }
        else {
            if (!descr.empty())
                descr.append("\r\n");

            descr.append(s, e - s);
        }

        s = e;
    }

    internal_error(true, "WEBRTC: setting remote sdp: %s", descr.c_str());
    rc->pc->setRemoteDescription(descr);

    s = candidates.c_str();
    while(*s) {
        if(*s == '\r' || *s == '\n') {
            s++;
            continue;
        }

        e = s;
        while(*e && *e != '\r' && *e != '\n')
            e++;

        std::string candidate;
        candidate.append(s, e - s);
        internal_error(true, "WEBRTC: adding remote candidate: %s", candidate.c_str());
        rc->pc->addRemoteCandidate(candidate);

        s = e;
    }

    bool logged = false;
    while(rc->gstate != rtc::PeerConnection::GatheringState::Complete) {
        if(!logged) {
            logged = true;
            internal_error(true, "WEBRTC: Waiting for gathering to complete");
        }
        usleep(1000);
    }

    if(logged)
        internal_error(true, "WEBRTC: Gathering complete");

    return rc;
}

struct webrtc_answer *webrtc_get_answer(void *webrtc_conn) {
    auto rc = (webrtc_connection *)webrtc_conn;
    return &rc->answer;
}

void webrtc_close(void *webrtc_conn) {
    auto rc = (webrtc_connection *)webrtc_conn;

    for(size_t i = 0; i < rc->unsafe.data_channel_id ; i++)
        if (rc->unsafe.dc[i])
            rc->unsafe.dc[i]->close();

    if (rc->pc)
        rc->pc->close();

    for(size_t i = 0; i < rc->answer.candidates_id ; i++)
        freez(rc->answer.candidates[i]);

    for(size_t i = 0; i < rc->answer.description_id ; i++)
        freez(rc->answer.description[i]);

    freez(rc);
}
