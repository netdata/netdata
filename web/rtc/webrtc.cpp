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
#ifdef NETDATA_INTERNAL_CHECKS
    rtc::InitLogger(rtc::LogLevel::Verbose);
#else
    rtc::InitLogger(rtc::LogLevel::Warning);
#endif
    rtc::Preload();
}

class webRTCConnection {
public:
    webRTCConnection(const char *sdp, BUFFER *wb, char **candidates, size_t *candidates_max);
    ~webRTCConnection();
    void close();
    bool shouldBeDeleted() const {
        internal_error(true, "WEBRTC[%zu]: should_be_deleted: %s", id, should_be_deleted ? "true" : "false");
        return should_be_deleted;
    };
    size_t ID() const {
        return id;
    };

private:
    size_t id;
    size_t version;
    rtc::Configuration config;
    std::shared_ptr<rtc::PeerConnection> pc;

    bool should_be_deleted;

    struct {
        SPINLOCK spinlock;
        size_t active;
        size_t data_channel_id;
        std::shared_ptr<rtc::DataChannel> dc[DATACHANNEL_ENTRIES_MAX];
    } unsafe;

    void applyRemoteSDP(const char *sdp);
    void getID();
    void increaseVersion();
};

void webRTCConnection::getID() {
    static size_t global_id = 0;
    id = __atomic_fetch_add(&global_id, 1, __ATOMIC_RELAXED);
}

void webRTCConnection::increaseVersion() {
    __atomic_add_fetch(&version, 1, __ATOMIC_RELAXED);
}

webRTCConnection::webRTCConnection(const char *sdp, BUFFER *wb, char **candidates, size_t *candidates_max) {
    getID();
    version = 0;
    should_be_deleted = false;

    netdata_spinlock_init(&unsafe.spinlock);
    unsafe.active = 0;
    unsafe.data_channel_id = 0;

    size_t candidate_id = 0;

    buffer_flush(wb);

    netdata_spinlock_init(&unsafe.spinlock);

    config.iceServers.emplace_back("stun.l.google.com:19302");
    config.iceTransportPolicy = rtc::TransportPolicy::All;
    config.enableIceTcp = true;
    config.enableIceUdpMux = true;
    config.maxMessageSize = 5 * 1024 * 1024;

    pc = std::make_shared<rtc::PeerConnection>(config);

    pc->onLocalDescription([&](rtc::Description description) {
        internal_error(true, "WEBRTC[%zu]: DESCRIPTION: %s", id, std::string(description).c_str());
        buffer_strcat(wb, std::string(description).c_str());
    });

    pc->onLocalCandidate([&](rtc::Candidate candidate) {
        internal_error(true, "WEBRTC[%zu]: CANDIDATE %zu: %s", id, candidate_id, std::string(candidate).c_str());

        if (candidate_id < *candidates_max)
            candidates[candidate_id++] = strdupz(std::string(candidate).c_str());
        else
            internal_error(true, "WEBRTC[%zu]: max candidates size of %zu reached.", id, candidate_id);
    });

    pc->onStateChange([&](rtc::PeerConnection::State pcstate) {
        increaseVersion();

        switch(pcstate) {
            case rtc::PeerConnection::State::New:
                internal_error(true, "WEBRTC[%zu]: STATE: New", id);
                break;

            case rtc::PeerConnection::State::Connecting:
                internal_error(true, "WEBRTC[%zu]: STATE: Connecting", id);
                break;

            case rtc::PeerConnection::State::Connected:
                internal_error(true, "WEBRTC[%zu]: STATE: Connected", id);
                break;

            case rtc::PeerConnection::State::Closed:
                internal_error(true, "WEBRTC[%zu]: STATE: Closed", id);
                should_be_deleted = true;
                break;

            case rtc::PeerConnection::State::Failed:
                internal_error(true, "WEBRTC[%zu]: STATE: Failed", id);
                should_be_deleted = true;
                break;

            case rtc::PeerConnection::State::Disconnected:
                internal_error(true, "WEBRTC[%zu]: STATE: Disconnected", id);
                should_be_deleted = true;
                break;
        }
    });

    pc->onGatheringStateChange([&](rtc::PeerConnection::GatheringState pcgstate) {
        increaseVersion();

        switch(pcgstate) {
            case rtc::PeerConnection::GatheringState::New:
                internal_error(true, "WEBRTC[%zu]: GATHERING STATE: New", id);
                break;

            case rtc::PeerConnection::GatheringState::InProgress:
                internal_error(true, "WEBRTC[%zu]: GATHERING STATE: InProgress", id);
                break;

            case rtc::PeerConnection::GatheringState::Complete:
                internal_error(true, "WEBRTC[%zu]: GATHERING STATE: Complete", id);
                break;
        }
    });

    pc->onSignalingStateChange([&](rtc::PeerConnection::SignalingState pcsstate) {
        increaseVersion();

        switch(pcsstate) {
            case rtc::PeerConnection::SignalingState::HaveLocalOffer:
                internal_error(true, "WEBRTC[%zu]: SIGNALING STATE: HaveLocalOffer", id);
                break;

            case rtc::PeerConnection::SignalingState::HaveRemoteOffer:
                internal_error(true, "WEBRTC[%zu]: SIGNALING STATE: HaveRemoteOffer", id);
                break;

            case rtc::PeerConnection::SignalingState::HaveLocalPranswer:
                internal_error(true, "WEBRTC[%zu]: SIGNALING STATE: HaveLocalPranswer", id);
                break;

            case rtc::PeerConnection::SignalingState::HaveRemotePranswer:
                internal_error(true, "WEBRTC[%zu]: SIGNALING STATE: HaveRemotePranswer", id);
                break;

            case rtc::PeerConnection::SignalingState::Stable:
                internal_error(true, "WEBRTC[%zu]: SIGNALING STATE: Stable", id);
                break;
        }
    });

    pc->onDataChannel([&](shared_ptr<rtc::DataChannel> _dc) {
        internal_error(true, "WEBRTC[%zu]: DATA CHANNEL '%s' OPEN", id, _dc->label().c_str());
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
                internal_error(true, "WEBRTC[%zu]: max data channels size %zu reached.", id, unsafe.data_channel_id);
            else
                unsafe.dc[unsafe.data_channel_id++] = _dc;
        }
        netdata_spinlock_unlock(&unsafe.spinlock);

        _dc->onClosed([&]()  {
                if(!should_be_deleted) {
                    increaseVersion();

                    internal_error(true, "WEBRTC[%zu]: DATA CHANNEL CLOSED", id);

                    netdata_spinlock_lock(&unsafe.spinlock);
                    unsafe.active--;
                    bool destroy = (unsafe.active == 0);

                    for (size_t i = 0; i < unsafe.data_channel_id; i++)
                        if (unsafe.dc[i] == _dc)
                            unsafe.dc[i] = nullptr;

                    netdata_spinlock_unlock(&unsafe.spinlock);

                    if (destroy)
                        should_be_deleted = true;
                }
            });

        _dc->onMessage([&](auto data) {
            if (std::holds_alternative<std::string>(data)) {
                rtc::DataChannel *dc = _dc.get();

                internal_error(true, "WEBRTC[%zu]: DATA CHANNEL '%s' MSG (max size %zu): %s", id, dc->label().c_str(),
                               dc->maxMessageSize(), std::get<std::string>(data).c_str());

                std::string response = std::get<std::string>(data);
                response.append(" to you too");
                dc->send(response.c_str());
            }
        });
    });

    applyRemoteSDP(sdp);

    bool logged = false;
    while(pc->gatheringState() != rtc::PeerConnection::GatheringState::Complete) {
        if(!logged) {
            logged = true;
            internal_error(true, "WEBRTC[%zu]: Waiting for gathering to complete", id);
        }
        usleep(1000);
    }

    if(logged)
        internal_error(true, "WEBRTC[%zu]: Gathering complete", id);

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

    internal_error(true, "WEBRTC[%zu]: setting remote sdp: %s", id, descr.c_str());
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
        internal_error(true, "WEBRTC[%zu]: adding remote candidate: %s", id, candidate.c_str());
        pc->addRemoteCandidate(candidate);

        s = e;
    }
}

void webRTCConnection::close() {
    internal_error(true, "WEBRTC[%zu]: closing...", id);

    for (size_t i = 0; i < unsafe.data_channel_id; i++) {
        if (unsafe.dc[i]) {
            internal_error(true, "WEBRTC[%zu]: DATA CHANNEL[%zu]: closing...", id, i);
            unsafe.dc[i]->close();
        }
    }

    if (pc) {
        pc->close();
    }
}

webRTCConnection::~webRTCConnection() {
    close();
    internal_error(true, "WEBRTC[%zu]: freeing...", id);
}

std::vector<webRTCConnection *> connections;

static void cleanupConnections() {
    internal_error(true, "WEBRTC: cleanupConnections called, size: %zu", connections.size());

    connections.erase(
            std::remove_if(connections.begin(), connections.end(),
                           [&](const webRTCConnection *conn) {
                               if (conn->shouldBeDeleted()) {
                                   delete conn;
                                   return true;
                               }
                               return false;
                           }),
            connections.end());
}

int webrtc_new_connection(const char *sdp, BUFFER *wb, char **candidates, size_t *candidates_max) {

    cleanupConnections();

    if(!sdp || !*sdp) {
        buffer_flush(wb);
        buffer_strcat(wb, "No SDP message posted with the request");
        wb->content_type = CT_TEXT_PLAIN;
        *candidates_max = 0;
        return HTTP_RESP_BAD_REQUEST;
    }

    auto connection = new webRTCConnection(sdp, wb, candidates, candidates_max);

    // Log the connection object details
    internal_error(true, "WEBRTC: Adding new connection, id: %zu, pointer: %p", connection->ID(), connection);

    // Log the connections vector size before adding the connection
    internal_error(true, "WEBRTC: connections.size() before push_back: %zu", connections.size());

    // Store the weak_ptr to the connection in a container.
    connections.push_back(connection);

    // Log the connections vector size after adding the connection
    internal_error(true, "WEBRTC: connections.size() after push_back: %zu", connections.size());

    return HTTP_RESP_OK;
}

void webrtc_close_all_connections() {
    rtc::Cleanup();
}

#else // ! HAVE_LIBDATACHANNEL

void webrtc_initialize() {
    ;
}

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
