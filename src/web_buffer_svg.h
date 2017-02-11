#ifndef NETDATA_WEB_BUFFER_SVG_H
#define NETDATA_WEB_BUFFER_SVG_H 1

/**
 * @file web_buffer_svg.h
 * @brief API to build a badage.svg
 */


/**
 * Build a badage.svg
 *
 * @param wb Web buffer to write to.
 * @param label of the badage
 * @param value of the badage
 * @param units of the badage
 * @param label_color of the badage
 * @param value_color of the badage
 * @param precision to print value with
 */
extern void buffer_svg(BUFFER *wb, const char *label, calculated_number value, const char *units, const char *label_color, const char *value_color, int precision);

/**
 * Write fromatted value and unit to `value_string`
 *
 * For a duration <= one second we print `now`.
 * For an infinite value we print `never`.
 *
 * If unit is one of `ok/error`, `ok/failed`, `up/down`, `on/off` we interpret value as boolean and print the representating name. I.e. `ok`
 *
 * If an number is printed set `precision` to the number of fractional digits the value should have.
 * A negative number means auto:
 *
 * value    | digits
 * -------- | --------
 * `> 1000` | 0
 * `> 10`   | 1
 * `> 0.1`  | 2 
 * `<= 0.1` | 4
 *
 * @param value_string to write to
 * @param value_string_len Length of `value_string`.
 * @param value to format
 * @param units to format
 * @param precision Number of fractional digits.
 * @return the formatted string
 */
extern char *format_value_and_unit(char *value_string, size_t value_string_len, calculated_number value, const char *units, int precision);

#endif /* NETDATA_WEB_BUFFER_SVG_H */
