// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_kmeans.h"
#include "libnetdata/libnetdata.h"
#include "dlib/dlib/clustering.h"

void
ml_kmeans_init(ml_kmeans_t *kmeans)
{
    kmeans->cluster_centers.reserve(2);
    kmeans->cluster_centers.clear();
    kmeans->min_dist = std::numeric_limits<calculated_number_t>::max();
    kmeans->max_dist = std::numeric_limits<calculated_number_t>::min();
}

void
ml_kmeans_train(ml_kmeans_t *kmeans, const ml_features_t *features, unsigned max_iters, time_t after, time_t before)
{
    kmeans->after = (uint32_t) after;
    kmeans->before = (uint32_t) before;

    kmeans->min_dist = std::numeric_limits<calculated_number_t>::max();
    kmeans->max_dist  = std::numeric_limits<calculated_number_t>::min();

    kmeans->cluster_centers.clear();

    dlib::pick_initial_centers(2, kmeans->cluster_centers, features->preprocessed_features);
    dlib::find_clusters_using_kmeans(features->preprocessed_features, kmeans->cluster_centers, max_iters);

    for (const auto &preprocessed_feature : features->preprocessed_features) {
        calculated_number_t mean_dist = 0.0;

        for (const auto &cluster_center : kmeans->cluster_centers) {
            mean_dist += dlib::length(cluster_center - preprocessed_feature);
        }

        mean_dist /= kmeans->cluster_centers.size();

        if (mean_dist < kmeans->min_dist)
            kmeans->min_dist = mean_dist;

        if (mean_dist > kmeans->max_dist)
            kmeans->max_dist = mean_dist;
    }
}

calculated_number_t
ml_kmeans_anomaly_score(const ml_kmeans_inlined_t *inlined_km, const DSample &DS)
{
    calculated_number_t mean_dist = 0.0;
    for (const auto &CC: inlined_km->cluster_centers)
        mean_dist += dlib::length(CC - DS);

    mean_dist /= inlined_km->cluster_centers.size();

    if (inlined_km->max_dist == inlined_km->min_dist)
        return 0.0;

    calculated_number_t anomaly_score = 100.0 * std::abs((mean_dist - inlined_km->min_dist) / (inlined_km->max_dist - inlined_km->min_dist));
    return (anomaly_score > 100.0) ? 100.0 : anomaly_score;
}

void
ml_kmeans_serialize(const ml_kmeans_inlined_t *inlined_km, BUFFER *wb)
{
    buffer_json_member_add_uint64(wb, "after", inlined_km->after);
    buffer_json_member_add_uint64(wb, "before", inlined_km->before);

    buffer_json_member_add_double(wb, "min_dist", inlined_km->min_dist);
    buffer_json_member_add_double(wb, "max_dist", inlined_km->max_dist);

    buffer_json_member_add_array(wb, "cluster_centers");
    for (const auto &cc: inlined_km->cluster_centers) {
        buffer_json_add_array_item_array(wb);

        for (const auto &d: cc) {
            buffer_json_add_array_item_double(wb, d);
        }

        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);
}

bool ml_kmeans_deserialize(ml_kmeans_inlined_t *inlined_km, struct json_object *root)
{
    struct json_object *value;

    if (!json_object_object_get_ex(root, "after", &value) ||
        !json_object_is_type(value, json_type_int)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse int for 'after'");
        return false;
    }
    inlined_km->after = json_object_get_int(value);

    if (!json_object_object_get_ex(root, "before", &value) ||
        !json_object_is_type(value, json_type_int)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse int for 'before'");
        return false;
    }
    inlined_km->before = json_object_get_int(value);

    if (!json_object_object_get_ex(root, "min_dist", &value) ||
        !json_object_is_type(value, json_type_double)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse double for 'min_dist'");
        return false;
    }
    inlined_km->min_dist = json_object_get_double(value);

    if (!json_object_object_get_ex(root, "max_dist", &value) ||
        !json_object_is_type(value, json_type_double)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse double for 'max_dist'");
        return false;
    }
    inlined_km->max_dist = json_object_get_double(value);

    struct json_object *cc_root;
    if (!json_object_object_get_ex(root, "cluster_centers", &cc_root) ||
        !json_object_is_type(cc_root, json_type_array)) {
        netdata_log_error("Failed to deserialize kmeans: failed to parse array for 'cluster_centers'");
        return false;
    }

    size_t num_centers = json_object_array_length(cc_root);
    if (num_centers != 2) {
        netdata_log_error("Failed to deserialize kmeans: expected cluster centers array of size 2");
        return false;
    }

    for (size_t i = 0; i < num_centers; i++) {
        struct json_object *cc_obj = json_object_array_get_idx(cc_root, i);
        if (!cc_obj || !json_object_is_type(cc_obj, json_type_array)) {
            netdata_log_error("Failed to deserialize kmeans: expected cluster center array");
            return false;
        }

        size_t size = json_object_array_length(cc_obj);
        if (size != 6) {
            netdata_log_error("Failed to deserialize kmeans: expected cluster center array of size 6");
            return false;
        }

        inlined_km->cluster_centers[i].set_size(size);
        for (size_t j = 0; i < size; j++) {
            struct json_object *value = json_object_array_get_idx(cc_obj, j);
            if (!value || !json_object_is_type(value, json_type_double))
                netdata_log_error("Failed to deserialize kmeans: failed to parse double %zu for cluster center %zu", j, i);
                return false;

            inlined_km->cluster_centers[i](j) = json_object_get_double(value);
        }
    }

    return true;
}
