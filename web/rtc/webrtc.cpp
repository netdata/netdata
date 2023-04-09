// SPDX-License-Identifier: GPL-3.0-or-later

#include "webrtc.h"

#include "../server/web_client.h"

#ifdef HAVE_LIBDATACHANNEL

#include "rtc/rtc.hpp"
#include <chrono>
#include <memory>
#include <vector>
#include <thread>

using namespace std::chrono_literals;
using std::shared_ptr;
using std::weak_ptr;
template <class T> weak_ptr<T> make_weak_ptr(shared_ptr<T> ptr) { return ptr; }

#define DATACHANNEL_ENTRIES_MAX 100

void webrtc_initialize() {
    rtc::InitLogger(rtc::LogLevel::Warning);
}

class webRTCConnection : public std::enable_shared_from_this<webRTCConnection> {
public:
    webRTCConnection(const char *sdp, BUFFER *wb, char **candidates, size_t *candidates_max);
    ~webRTCConnection();
    void close();

private:
    size_t version;
    rtc::Configuration config;
    std::shared_ptr<rtc::PeerConnection> pc;
    rtc::PeerConnection::State state;
    rtc::PeerConnection::GatheringState gstate;

    struct {
        SPINLOCK spinlock;
        size_t active;
        size_t data_channel_id;
        std::shared_ptr<rtc::DataChannel> dc[DATACHANNEL_ENTRIES_MAX];
    } unsafe;

    void applyRemoteSDP(const char *sdp);
    void increaseVersion();
};

void webRTCConnection::increaseVersion() {
    __atomic_add_fetch(&version, 1, __ATOMIC_RELAXED);
}

webRTCConnection::webRTCConnection(const char *sdp, BUFFER *wb, char **candidates, size_t *candidates_max) {
    version = 0;

    netdata_spinlock_init(&unsafe.spinlock);
    unsafe.active = 0;
    unsafe.data_channel_id = 0;

    size_t candidate_id = 0;

    buffer_flush(wb);

    netdata_spinlock_init(&unsafe.spinlock);

    config.iceServers.emplace_back("stun.l.google.com:19302");
    pc = std::make_shared<rtc::PeerConnection>(config);

    pc->onLocalDescription([&](rtc::Description description) {
        internal_error(true, "WEBRTC: DESCRIPTION: %s", std::string(description).c_str());
        buffer_strcat(wb, std::string(description).c_str());
    });

    pc->onLocalCandidate([&](rtc::Candidate candidate) {
        internal_error(true, "WEBRTC: CANDIDATE %zu: %s", candidate_id, std::string(candidate).c_str());

        if (candidate_id < *candidates_max) {
            candidates[candidate_id++] = strdupz(std::string(candidate).c_str());
        }
        else
            internal_error(true, "WEBRTC: max candidates size reached.");
    });

    pc->onStateChange([&](rtc::PeerConnection::State pcstate) {
        internal_error(true, "WEBRTC: STATE: %d", (int)pcstate);
        increaseVersion();
        state = pcstate;
    });
    pc->onGatheringStateChange([&](rtc::PeerConnection::GatheringState pcgstate) {
        internal_error(true, "WEBRTC: GATHERING STATE: %d", (int)pcgstate);
        increaseVersion();
        gstate = pcgstate;
    });

    pc->onDataChannel([&](shared_ptr<rtc::DataChannel> _dc) {
        internal_error(true, "WEBRTC: DATA CHANNEL '%s' OPEN", _dc->label().c_str());

        increaseVersion();
        netdata_spinlock_lock(&unsafe.spinlock);
        unsafe.active++;
        bool found = false;
        for(size_t i = 0; i < unsafe.data_channel_id ; i++)
            if(!unsafe.dc[i]) {
                unsafe.dc[unsafe.data_channel_id++] = _dc;
                found = true;
            }

        if(!found) {
            if (unsafe.data_channel_id >= DATACHANNEL_ENTRIES_MAX)
                internal_fatal(true, "WEBRTC: max data channels size reached.");

            unsafe.dc[unsafe.data_channel_id++] = _dc;
        }
        netdata_spinlock_unlock(&unsafe.spinlock);

        _dc->onClosed(
                [&, connection = shared_from_this()]() {
                    increaseVersion();

                    internal_error(true, "WEBRTC: DATA CHANNEL '%s' CLOSED", _dc->label().c_str());
                    netdata_spinlock_lock(&unsafe.spinlock);
                    unsafe.active--;
                    for(size_t i = 0; i < unsafe.data_channel_id ; i++)
                        if(unsafe.dc[i] == _dc)
                            unsafe.dc[i] = nullptr;

                    netdata_spinlock_unlock(&unsafe.spinlock);

                    // No need to call 'delete this' as the shared_ptr will handle object destruction.
                    // If 'destroy' is true and this was the last reference to the object, the object will be destroyed.
                });

        _dc->onMessage([&](auto data) {
            if (std::holds_alternative<std::string>(data)) {
                internal_error(true, "WEBRTC: DATA CHANNEL '%s' MSG: %s", _dc->label().c_str(),
                               std::get<std::string>(data).c_str());
            }
        });
    });

    bool logged = false;
    while(gstate != rtc::PeerConnection::GatheringState::Complete) {
        if(!logged) {
            logged = true;
            internal_error(true, "WEBRTC: Waiting for gathering to complete");
        }
        usleep(1000);
    }

    if(logged)
        internal_error(true, "WEBRTC: Gathering complete");


    applyRemoteSDP(sdp);
    *candidates_max = candidate_id;
}

void webRTCConnection::applyRemoteSDP(const char *sdp) {
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
    pc->setRemoteDescription(descr);

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
        pc->addRemoteCandidate(candidate);

        s = e;
    }
}

void webRTCConnection::close() {
    for (size_t i = 0; i < unsafe.data_channel_id; i++) {
        if (unsafe.dc[i]) {
            unsafe.dc[i]->close();
        }
    }

    if (pc) {
        pc->close();
    }
}

webRTCConnection::~webRTCConnection() {
    close();
}

std::vector<std::weak_ptr<webRTCConnection>> connections;

int webrtc_new_connection(const char *sdp, BUFFER *wb, char **candidates, size_t *candidates_max) {
    auto connection = std::make_shared<webRTCConnection>(sdp, wb, candidates, candidates_max);
    // The connection object will self-destruct when the last data channel closes, and the shared_ptr's reference count reaches 0.

    // Store the weak_ptr to the connection in a container.
    // Replace "connections" with a container that suits your needs.
    connections.push_back(connection);

    return HTTP_RESP_OK;
}

void webrtc_close_all_connections() {
    for (auto &weak_connection : connections) {
        if (auto connection = weak_connection.lock()) {
            connection->close();
        }
    }
    connections.clear();
}

#else // ! HAVE_LIBDATACHANNEL

int webrtc_new_connection(const char *sdp __maybe_unused, BUFFER *wb, char **candidates __maybe_unused, size_t *candidates_max) {
    buffer_flush(wb);
    buffer_strcat(wb, "WEBRTC is not enabled on this server");
    wb->content_type = CT_TEXT_PLAIN;
    *candidates_max = 0;
    return HTTP_RESP_BAD_REQUEST;
}

void webrtc_close_all_connections() {
    ;
}

#endif // ! HAVE_LIBDATACHANNEL
