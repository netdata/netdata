#include "replication-private.h"
#include "Logger.h"

using namespace replication;

class Host {
public:
    Host(RRDHOST *RH) : RH(RH), L(rrdhost_hostname(RH)) {}

    void setReceiverGaps(std::vector<TimeRange> &TRs) {
        std::lock_guard<Mutex> Lock(ReceiverMutex);
        ReceiverGaps = TRs;
    }

    std::vector<TimeRange> getReceiverGaps() {
        std::lock_guard<Mutex> Lock(ReceiverMutex);
        return ReceiverGaps;
    }

    void setSenderGaps(std::vector<TimeRange> &TRs) {
        std::lock_guard<Mutex> Lock(SenderMutex);
        SenderGaps = TRs;
    }

    std::vector<TimeRange> getSenderGaps() {
        std::lock_guard<Mutex> Lock(SenderMutex);
        return SenderGaps;
    }

    void startReplicationThread() {
        ReplicationThread = std::thread(&Host::senderReplicateGaps, this);
    }

    void stopReplicationThread() {
        netdata_thread_cancel(ReplicationThread.native_handle());
        ReplicationThread.join();

        time_t FirstEntryT = rrdhost_first_entry_t(RH);
        time_t LastEntryT = rrdhost_last_entry_t(RH);
        replication_save_host_entries_range(&RH->host_uuid, FirstEntryT, LastEntryT);

        error("GVD[stopReplicationThread]: %s entries [%ld, %ld]",
              rrdhost_hostname(RH), FirstEntryT, LastEntryT);
    }

    /* adds a new gap */
    void receiverConnect() {
        std::lock_guard<Mutex> Lock(ReceiverMutex);

        time_t CurrT = now_realtime_sec();

        time_t FirstEntryT = 0, LastEntryT = 0;
        replication_load_host_entries_range(&RH->host_uuid, &FirstEntryT, &LastEntryT);

        if (LastEntryT == 0)
            LastEntryT = CurrT - Cfg.SecondsToReplicateOnFirstConnection + 1;
        else
            LastEntryT -= maxUpdateEvery();

        if (LastEntryT >= CurrT) {
            error("[%s] Skipping invalid replication time range on connect: <%ld, %ld>",
                  rrdhost_hostname(RH), LastEntryT, CurrT);
            return;
        }

        TimeRange TR(LastEntryT, CurrT + 60);
        std::vector<TimeRange> NewTRs = splitTimeRange(TR, Cfg.MaxEntriesPerGapData);
        for (const TimeRange &NewTR : NewTRs)
            ReceiverGaps.push_back(NewTR);

        ReceiverGaps = coalesceTimeRanges(ReceiverGaps);
    }

    /* drops a received gap */
    void receiverDropGap(const TimeRange &TR) {
        std::lock_guard<Mutex> Lock(ReceiverMutex);
        ReceiverGaps.erase(std::remove(ReceiverGaps.begin(), ReceiverGaps.end(), TR), ReceiverGaps.end());
    }

    size_t receiverNumberOfGaps() {
        std::lock_guard<Mutex> Lock(ReceiverMutex);
        return ReceiverGaps.size();
    }

    /* replicate gaps */
    void senderReplicateGaps() {
        while (!netdata_exit) {
            /*
             * Sleep while we don't have any gaps to fill.
             */
            size_t NumGaps = 0;
            while (NumGaps == 0) {
                {
                    std::lock_guard<Mutex> Lock(SenderMutex);
                    NumGaps = SenderGaps.size();
                }

                std::this_thread::sleep_for(std::chrono::seconds(1));
            }

            /*
             * Find the next gap we want to process.
             */
            TimeRange Gap;
            {
                std::lock_guard<Mutex> Lock(SenderMutex);
                if (SenderGaps.size() == 0)
                    continue;
                Gap = SenderGaps.back();
            }

            /*
             * Create a vector that will contain the list of dimensions that
             * we want to fill for this gap. Right now, we consider only
             * dimensions that are in mem.
             */
            std::vector<GapData> GDV;

            void *VRS;
            rrdset_foreach_read(VRS, RH) {
                RRDSET *RS = static_cast<RRDSET *>(VRS);

                void *VRD;
                rrddim_foreach_read(VRD, RS) {
                    RRDDIM *RD = static_cast<RRDDIM *>(VRD);
                    GapData GD(rrdset_id(RS), rrddim_id(RD));
                    GDV.push_back(GD);
                }
                rrddim_foreach_done(VRD);
            }
            rrdset_foreach_done(VRS);

            /*
             * Sleep enough time to let the streaming thread push
             * chart defs & 1st values of dims.
             */
            time_t MaxUE = 2 * maxUpdateEvery();
            error("[%s]: sleeping for 2 * max update_every=%ld", rrdhost_hostname(RH), MaxUE);
            std::this_thread::sleep_for(std::chrono::seconds(MaxUE));

            /*
             * Start sending the gap data for each individual dimension
             */
            RateLimiter RL(Cfg.MaxQueriesPerSecond, std::chrono::seconds(1));
            for (GapData &GD : GDV) {
                RL.request();

                /*
                 * Sleep while we are receiving gaps for this host
                 */
                while (!netdata_exit) {
                    size_t NumReceiverGaps = 0;
                    {
                        std::lock_guard<Mutex> Lock(ReceiverMutex);
                        NumReceiverGaps = ReceiverGaps.size();
                    }

                    if (!NumReceiverGaps)
                        break;

                    error("[%s] Replication thread sleeping because we are receiving gaps", rrdhost_hostname(RH));
                    std::this_thread::sleep_for(std::chrono::seconds(1));
                }

                /*
                 * Find the dim we are interested in and query it.
                 */
                rrdhost_rdlock(RH);
                RRDSET *RS = rrdset_find(RH, GD.getChart().c_str());
                if (!RS) {
                    error("[%s] Could not find chart %s for dim %s to fill <%ld, %ld>",
                          rrdhost_hostname(RH), GD.getChart().c_str(), GD.getDimension().c_str(), Gap.first, Gap.second);
                    rrdhost_unlock(RH);
                    continue;
                }

                rrdset_rdlock(RS);

                if (!rrdset_push_chart_definition_now(RS)) {
                    /* We shouldn't check this chart upstream. Unlock the
                     * chart/host and continue with the next entry in the
                     * GapData vector */
                    rrdset_unlock(RS);
                    rrdhost_unlock(RH);
                    continue;
                }

                RRDDIM *RD = rrddim_find(RS, GD.getDimension().c_str());
                if (!RS) {
                    error("[%s] Could not find dim %s.%s to fill <%ld, %ld>",
                          rrdhost_hostname(RH), GD.getChart().c_str(), GD.getDimension().c_str(), Gap.first, Gap.second);
                    rrdset_unlock(RS);
                    rrdhost_unlock(RH);
                    continue;
                }

                // Fill data for this dimension
                std::pair<std::vector<time_t>, std::vector<storage_number>> P =
                    Query::getSNs(RD, Gap.first, Gap.second);
                GD.setPayload(std::move(P.first), std::move(P.second));

                rrdset_unlock(RS);
                rrdhost_unlock(RH);

                /*
                 * Try to send the data upstream
                 */
                while (!GD.push(RH->sender)) {
                    error("[%s] Sender buffer is full (Dim=%s.%s, Gap=<%ld, %ld>)",
                          rrdhost_hostname(RH), GD.getChart().c_str(), GD.getDimension().c_str(), Gap.first, Gap.second);
                    std::this_thread::sleep_for(std::chrono::seconds(1));
                }

                L.senderFilledGap(GD);
            }

            /*
             * Now that we filled this gap, send a DROPGAP command to let
             * the parent know that we have no more data to send
             */
            sender_start(RH->sender);
            buffer_sprintf(RH->sender->build, "DROPGAP \"%ld\" \"%ld\"\n", Gap.first, Gap.second);
            sender_commit(RH->sender);

            error("[%s] Sent DROPGAP command for time range <%ld, %ld>",
                  rrdhost_hostname(RH), Gap.first, Gap.second);

            /*
             * Nothing else to do... Just remove the gap
             */
            {
                std::lock_guard<Mutex> Lock(SenderMutex);
                SenderGaps.erase(std::remove(SenderGaps.begin(), SenderGaps.end(), Gap), SenderGaps.end());
            }
            L.senderDroppedGap(Gap);
        }
    }

    time_t maxUpdateEvery() const {
        rrdhost_rdlock(RH);
        time_t MaxUE = RH->rrd_update_every;

        void *VRS;

        rrdset_foreach_read(VRS, RH) {
            RRDSET *RS = static_cast<RRDSET *>(VRS);
            MaxUE = std::max<time_t>(RS->update_every, MaxUE);

            void *VRD;
            rrddim_foreach_read(VRD, RS) {
                RRDDIM *RD = static_cast<RRDDIM *>(VRD);
                MaxUE = std::max<time_t>(RD->update_every, MaxUE);
            }
            rrddim_foreach_done(VRD);
        }
        rrdset_foreach_done(VRS);

        return MaxUE;
    }

    const char *getLogs() {
        return strdupz(L.serialize().c_str());
    }

    Logger &getLogger() {
        return L;
    }

private:
    RRDHOST *RH;
    Logger L;

    Mutex ReceiverMutex;
    std::vector<TimeRange> ReceiverGaps;

    Mutex SenderMutex;
    std::vector<TimeRange> SenderGaps;

    std::thread ReplicationThread;
};


/*
 * C API
 */

void replication_init(void) {
    Cfg.readReplicationConfig();
}

void replication_fini(void) {
}

void replication_new_host(RRDHOST *RH) {
    if (!Cfg.EnableReplication)
        return;

    /*
     * Load receiver gaps from sqlite db
    */
    Host *H = new Host(RH);
    RH->repl_handle = static_cast<replication_handle_t>(H);
    replication_load_gaps(RH);
}

void replication_host_gaps_from_sqlite_blob(RRDHOST *RH, const char *Buf, size_t Len) {
    Host *H = static_cast<Host *>(RH->repl_handle);
    if (!H)
        return;

    std::vector<TimeRange> TRs = deserializeTimeRangesFromArray(Buf, Len);
    H->setReceiverGaps(TRs);

    /*
     * Log info
     */
    auto &L = H->getLogger();
    L.createdHost(TRs);
}

void replication_delete_host(RRDHOST *RH) {
    Host *H = static_cast<Host *>(RH->repl_handle);
    if (!H)
        return;

    /*
     * Save receiver gaps to sqlite DB
     */
    std::vector<TimeRange> TRs = H->getReceiverGaps();
    std::string Buf = serializeTimeRangesToString(TRs);
    replication_save_gaps(RH, Buf.data(), Buf.size());

    /*
     * Log info
     */
    auto &L = H->getLogger();
    L.deletedHost(TRs);

    /*
     * Delete host
     */
    delete H;
    RH->repl_handle = nullptr;
}

void replication_thread_start(RRDHOST *RH) {
    Host *H = static_cast<Host *>(RH->repl_handle);
    if (!H)
        return;

    H->startReplicationThread();

    /*
     * Log info
     */
    auto &L = H->getLogger();
    L.startedReplicationThread();
}

void replication_thread_stop(RRDHOST *RH) {
    Host *H = static_cast<Host *>(RH->repl_handle);
    if (!H)
        return;

    H->stopReplicationThread();

    /*
     * Log info
     */
    auto &L = H->getLogger();
    L.stoppedReplicationThread();
}

void replication_set_sender_gaps(RRDHOST *RH, const char *Buf, size_t Len) {
    Host *H = static_cast<Host *>(RH->repl_handle);
    if (!H)
        return;

    std::vector<TimeRange> TRs = deserializeTimeRangesFromArray(Buf, Len);

    /* Assign the recv'd gaps to the host. The parent sends the gaps
     * in increasing timestamp order; reverse the vector because
     * we always pop from the back */
    std::reverse(TRs.begin(), TRs.end());
    H->setSenderGaps(TRs);

    /*
     * Log info
     */
    auto &L = H->getLogger();
    L.senderReceivedGaps(TRs);
}

void replication_get_receiver_gaps(RRDHOST *RH, char **Buf, uint32_t *Len) {
    Host *H = static_cast<Host *>(RH->repl_handle);
    if (!H)
        return;

    H->receiverConnect();
    std::vector<TimeRange> TRs = H->getReceiverGaps();

    std::string protoBuf = serializeTimeRangesToString(TRs);

    *Buf = static_cast<char*>(callocz(sizeof(char), protoBuf.size()));
    memcpy(*Buf, protoBuf.data(), protoBuf.size());
    *Len = protoBuf.size();

    /*
     * Log info
     */
    auto &L = H->getLogger();
    L.receiverSentGaps(TRs);
}

bool replication_receiver_fill_gap(RRDHOST *RH, const char *Buf) {
    if (Cfg.EnableFillGapLogging) {
        char LogPath[FILENAME_MAX + 1];
        snprintfz(LogPath, FILENAME_MAX, "%s/%s.fg",
                netdata_configured_cache_dir, rrdhost_hostname(RH));
        FILE *FP = fopen(LogPath, "a");
        if (!FP) {
            error("Could not open log file: %s", LogPath);
            return false;
        }
        size_t BufLen = strlen(Buf);
        fprintf(FP, "%zu %s\n", BufLen, Buf);
        fclose(FP);
    }

    GapData GD = GapData::fromBase64(Buf);

    Host *H = static_cast<Host *>(RH->repl_handle);

    /*
     * Log info
     */
    Logger &L = H->getLogger();
    L.receiverFilledGap(GD);

    return GD.flushToDBEngine(RH);
}

void replication_receiver_drop_gap(RRDHOST *RH, time_t After, time_t Before) {
    Host *H = static_cast<Host *>(RH->repl_handle);
    if (!H)
        return;

    TimeRange TR(After, Before);
    H->receiverDropGap(TR);

    /*
     * Log info
     */
    auto &L = H->getLogger();
    L.receiverDroppedGap(TR);
}

size_t replication_receiver_number_of_pending_gaps(RRDHOST *RH) {
    Host *H = static_cast<Host *>(RH->repl_handle);
    if (!H)
        return 0;

    return H->receiverNumberOfGaps();
}

const char *replication_logs(RRDHOST *RH) {
    Host *H = static_cast<Host *>(RH->repl_handle);
    if (!H) {
        std::stringstream SS;
        SS << "Replication is not enabled for host " << RH->hostname;
        return strdupz(SS.str().c_str());
    }

    return H->getLogs();
}
