#ifndef NETDATA_STORAGE_NUMBER_H
#define NETDATA_STORAGE_NUMBER_H

/**
 * @file storage_number.h
 * @brief Round robin database data structures.
 *
 * \todo Describe how to use the API
 */

/// ktsaou: Your help needed
typedef long double calculated_number;
#define CALCULATED_NUMBER_FORMAT "%0.7Lf" ///< format string for calculated_number
//typedef long long calculated_number;
//#define CALCULATED_NUMBER_FORMAT "%lld"

/// ktsaou: Your help needed
typedef long long collected_number;
#define COLLECTED_NUMBER_FORMAT "%lld" ///< format string for collected_number

/*
typedef long double collected_number;
#define COLLECTED_NUMBER_FORMAT "%0.7Lf"
*/

/// ktsaou: Your help needed
typedef uint32_t storage_number;
#define STORAGE_NUMBER_FORMAT "%u" ///< format sring for storage_number

#define SN_NOT_EXISTS       (0x0 << 24) ///< ktsaou: Your help needed
#define SN_EXISTS           (0x1 << 24) ///< ktsaou: Your help needed
#define SN_EXISTS_RESET     (0x2 << 24) ///< ktsaou: Your help needed
#define SN_EXISTS_UNDEF1    (0x3 << 24) ///< ktsaou: Your help needed
#define SN_EXISTS_UNDEF2    (0x4 << 24) ///< ktsaou: Your help needed
#define SN_EXISTS_UNDEF3    (0x5 << 24) ///< ktsaou: Your help needed
#define SN_EXISTS_UNDEF4    (0x6 << 24) ///< ktsaou: Your help needed

#define SN_FLAGS_MASK       (~(0x6 << 24)) ///< ktsaou: Your help needed

/**
 * extract the flags
 *
 * @param value storage_number with flags
 * @return extracted flags
 */
#define get_storage_number_flags(value) ((((storage_number)value) & (1 << 24)) | (((storage_number)value) & (2 << 24)) | (((storage_number)value) & (4 << 24)))

/**
 * Check if storage number exists
 *
 * @param value to check
 * @return boolean
 */
#define does_storage_number_exist(value) ((get_storage_number_flags(value) != 0)?1:0)
/**
 * Check if storage number was reset.
 *
 * @param value to check
 * @return boolean
 */
#define did_storage_number_reset(value)  ((get_storage_number_flags(value) == SN_EXISTS_RESET)?1:0)

/**
 * Convert calculated_number `value` to a storage_number.
 *
 * Add `flags` to the result.
 *
 * @param value to convert
 * @param flags to set on the result
 * @return storage_number
 */
storage_number pack_storage_number(calculated_number value, uint32_t flags);
/**
 * Convert storage_number `value` to a calculated_number.
 *
 * @param value to convert
 * @return calculated_number
 */
calculated_number unpack_storage_number(storage_number value);

/**
 * Write calculated_number `value` into string `str`.
 *
 * @param str to write to
 * @param value to write
 * @return characters written
 */
int print_calculated_number(char *str, calculated_number value);

#define STORAGE_NUMBER_POSITIVE_MAX 167772150000000.0  ///< Maximum positive storage number
#define STORAGE_NUMBER_POSITIVE_MIN 0.00001            ///< Minimum positive storage number
#define STORAGE_NUMBER_NEGATIVE_MAX -0.00001           ///< Maximum negative storage number
#define STORAGE_NUMBER_NEGATIVE_MIN -167772150000000.0 ///< Minimum negative storage number

/// accepted accuracy loss
#define ACCURACY_LOSS 0.0001
/// ktsaou: Your help needed
#define accuracy_loss(t1, t2) ((t1 == t2 || t1 == 0.0 || t2 == 0.0) ? 0.0 : (100.0 - ((t1 > t2) ? (t2 * 100.0 / t1 ) : (t1 * 100.0 / t2))))

#endif /* NETDATA_STORAGE_NUMBER_H */
