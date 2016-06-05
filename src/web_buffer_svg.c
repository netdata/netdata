#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <math.h>
#include <string.h>
#include <ctype.h>
#include <stdlib.h>

#include "common.h"
#include "log.h"
#include "appconfig.h"
#include "url.h"
#include "web_buffer.h"
#include "web_buffer_svg.h"

#define BADGE_HORIZONTAL_PADDING 4
#define VERDANA_KERNING 0.5

/*
 * verdana11_widths[] has been generated with this method:
 * https://github.com/badges/shields/blob/master/measure-text.js
*/

double verdana11_widths[256] = {
	[0] = 0.0,
	[1] = 0.0,
	[2] = 0.0,
	[3] = 0.0,
	[4] = 0.0,
	[5] = 0.0,
	[6] = 0.0,
	[7] = 0.0,
	[8] = 0.0,
	[9] = 0.0,
	[10] = 0.0,
	[11] = 0.0,
	[12] = 0.0,
	[13] = 0.0,
	[14] = 0.0,
	[15] = 0.0,
	[16] = 0.0,
	[17] = 0.0,
	[18] = 0.0,
	[19] = 0.0,
	[20] = 0.0,
	[21] = 0.0,
	[22] = 0.0,
	[23] = 0.0,
	[24] = 0.0,
	[25] = 0.0,
	[26] = 0.0,
	[27] = 0.0,
	[28] = 0.0,
	[29] = 0.0,
	[30] = 0.0,
	[31] = 0.0,
	[32] = 3.8671874999999996, //
	[33] = 4.3291015625, // !
	[34] = 5.048828125, // "
	[35] = 9.001953125, // #
	[36] = 6.9931640625, // $
	[37] = 11.837890625, // %
	[38] = 7.992187499999999, // &
	[39] = 2.9541015625, // '
	[40] = 4.9951171875, // (
	[41] = 4.9951171875, // )
	[42] = 6.9931640625, // *
	[43] = 9.001953125, // +
	[44] = 4.00146484375, // ,
	[45] = 4.9951171875, // -
	[46] = 4.00146484375, // .
	[47] = 4.9951171875, // /
	[48] = 6.9931640625, // 0
	[49] = 6.9931640625, // 1
	[50] = 6.9931640625, // 2
	[51] = 6.9931640625, // 3
	[52] = 6.9931640625, // 4
	[53] = 6.9931640625, // 5
	[54] = 6.9931640625, // 6
	[55] = 6.9931640625, // 7
	[56] = 6.9931640625, // 8
	[57] = 6.9931640625, // 9
	[58] = 4.9951171875, // :
	[59] = 4.9951171875, // ;
	[60] = 9.001953125, // <
	[61] = 9.001953125, // =
	[62] = 9.001953125, // >
	[63] = 5.99951171875, // ?
	[64] = 11.0, // @
	[65] = 7.51953125, // A
	[66] = 7.541015625, // B
	[67] = 7.680664062499999, // C
	[68] = 8.4755859375, // D
	[69] = 6.95556640625, // E
	[70] = 6.32177734375, // F
	[71] = 8.529296875, // G
	[72] = 8.26611328125, // H
	[73] = 4.6298828125, // I
	[74] = 5.00048828125, // J
	[75] = 7.62158203125, // K
	[76] = 6.123046875, // L
	[77] = 9.2705078125, // M
	[78] = 8.228515625, // N
	[79] = 8.658203125, // O
	[80] = 6.63330078125, // P
	[81] = 8.658203125, // Q
	[82] = 7.6484375, // R
	[83] = 7.51953125, // S
	[84] = 6.7783203125, // T
	[85] = 8.05126953125, // U
	[86] = 7.51953125, // V
	[87] = 10.87646484375, // W
	[88] = 7.53564453125, // X
	[89] = 6.767578125, // Y
	[90] = 7.53564453125, // Z
	[91] = 4.9951171875, // [
	[92] = 4.9951171875, // backslash
	[93] = 4.9951171875, // ]
	[94] = 9.001953125, // ^
	[95] = 6.9931640625, // _
	[96] = 6.9931640625, // `
	[97] = 6.6064453125, // a
	[98] = 6.853515625, // b
	[99] = 5.73095703125, // c
	[100] = 6.853515625, // d
	[101] = 6.552734375, // e
	[102] = 3.8671874999999996, // f
	[103] = 6.853515625, // g
	[104] = 6.9609375, // h
	[105] = 3.0185546875, // i
	[106] = 3.78662109375, // j
	[107] = 6.509765625, // k
	[108] = 3.0185546875, // l
	[109] = 10.69921875, // m
	[110] = 6.9609375, // n
	[111] = 6.67626953125, // o
	[112] = 6.853515625, // p
	[113] = 6.853515625, // q
	[114] = 4.6943359375, // r
	[115] = 5.73095703125, // s
	[116] = 4.33447265625, // t
	[117] = 6.9609375, // u
	[118] = 6.509765625, // v
	[119] = 9.001953125, // w
	[120] = 6.509765625, // x
	[121] = 6.509765625, // y
	[122] = 5.779296875, // z
	[123] = 6.982421875, // {
	[124] = 4.9951171875, // |
	[125] = 6.982421875, // }
	[126] = 9.001953125, // ~
	[127] = 0.0,
	[128] = 0.0,
	[129] = 0.0,
	[130] = 0.0,
	[131] = 0.0,
	[132] = 0.0,
	[133] = 0.0,
	[134] = 0.0,
	[135] = 0.0,
	[136] = 0.0,
	[137] = 0.0,
	[138] = 0.0,
	[139] = 0.0,
	[140] = 0.0,
	[141] = 0.0,
	[142] = 0.0,
	[143] = 0.0,
	[144] = 0.0,
	[145] = 0.0,
	[146] = 0.0,
	[147] = 0.0,
	[148] = 0.0,
	[149] = 0.0,
	[150] = 0.0,
	[151] = 0.0,
	[152] = 0.0,
	[153] = 0.0,
	[154] = 0.0,
	[155] = 0.0,
	[156] = 0.0,
	[157] = 0.0,
	[158] = 0.0,
	[159] = 0.0,
	[160] = 0.0,
	[161] = 0.0,
	[162] = 0.0,
	[163] = 0.0,
	[164] = 0.0,
	[165] = 0.0,
	[166] = 0.0,
	[167] = 0.0,
	[168] = 0.0,
	[169] = 0.0,
	[170] = 0.0,
	[171] = 0.0,
	[172] = 0.0,
	[173] = 0.0,
	[174] = 0.0,
	[175] = 0.0,
	[176] = 0.0,
	[177] = 0.0,
	[178] = 0.0,
	[179] = 0.0,
	[180] = 0.0,
	[181] = 0.0,
	[182] = 0.0,
	[183] = 0.0,
	[184] = 0.0,
	[185] = 0.0,
	[186] = 0.0,
	[187] = 0.0,
	[188] = 0.0,
	[189] = 0.0,
	[190] = 0.0,
	[191] = 0.0,
	[192] = 0.0,
	[193] = 0.0,
	[194] = 0.0,
	[195] = 0.0,
	[196] = 0.0,
	[197] = 0.0,
	[198] = 0.0,
	[199] = 0.0,
	[200] = 0.0,
	[201] = 0.0,
	[202] = 0.0,
	[203] = 0.0,
	[204] = 0.0,
	[205] = 0.0,
	[206] = 0.0,
	[207] = 0.0,
	[208] = 0.0,
	[209] = 0.0,
	[210] = 0.0,
	[211] = 0.0,
	[212] = 0.0,
	[213] = 0.0,
	[214] = 0.0,
	[215] = 0.0,
	[216] = 0.0,
	[217] = 0.0,
	[218] = 0.0,
	[219] = 0.0,
	[220] = 0.0,
	[221] = 0.0,
	[222] = 0.0,
	[223] = 0.0,
	[224] = 0.0,
	[225] = 0.0,
	[226] = 0.0,
	[227] = 0.0,
	[228] = 0.0,
	[229] = 0.0,
	[230] = 0.0,
	[231] = 0.0,
	[232] = 0.0,
	[233] = 0.0,
	[234] = 0.0,
	[235] = 0.0,
	[236] = 0.0,
	[237] = 0.0,
	[238] = 0.0,
	[239] = 0.0,
	[240] = 0.0,
	[241] = 0.0,
	[242] = 0.0,
	[243] = 0.0,
	[244] = 0.0,
	[245] = 0.0,
	[246] = 0.0,
	[247] = 0.0,
	[248] = 0.0,
	[249] = 0.0,
	[250] = 0.0,
	[251] = 0.0,
	[252] = 0.0,
	[253] = 0.0,
	[254] = 0.0,
	[255] = 0.0
};

// find the width of the string using the verdana 11points font
// re-write the string in place, skiping zero-length characters
static inline int verdana11_width(char *s) {
	double w = 0.0;
	char *d = s;

	while(*s) {
		double t = verdana11_widths[(unsigned char)*s];
		if(t == 0.0)
			s++;
		else {
			w += t + VERDANA_KERNING;
			if(d != s)
				*d++ = *s++;
			else
				d = ++s;
		}
	}

	*d = '\0';
	w -= VERDANA_KERNING;
	return ceil(w);
}

static inline size_t escape_xmlz(char *dst, const char *src, size_t len) {
	size_t i = len;

	// required escapes from
	// https://github.com/badges/shields/blob/master/badge.js
	while(*src && i) {
		switch(*src) {
			case '\\':
				*dst++ = '/';
				src++;
				i--;
				break;

			case '&':
				if(i > 5) {
					strcpy(dst, "&amp;");
					i -= 5;
					dst += 5;
					src++;
				}
				else goto cleanup;
				break;

			case '<':
				if(i > 4) {
					strcpy(dst, "&lt;");
					i -= 4;
					dst += 4;
					src++;
				}
				else goto cleanup;
				break;

			case '>':
				if(i > 4) {
					strcpy(dst, "&gt;");
					i -= 4;
					dst += 4;
					src++;
				}
				else goto cleanup;
				break;

			case '"':
				if(i > 6) {
					strcpy(dst, "&quot;");
					i -= 6;
					dst += 6;
					src++;
				}
				else goto cleanup;
				break;

			case '\'':
				if(i > 6) {
					strcpy(dst, "&apos;");
					i -= 6;
					dst += 6;
					src++;
				}
				else goto cleanup;
				break;

			default:
				i--;
				*dst++ = *src++;
				break;
		}
	}

cleanup:
	*dst = '\0';
	return len - i;
}

static inline const char *fix_units(const char *units) {
	if(!units || !*units || !strcmp(units, "empty") || !strcmp(units, "null")) return "";
	if(!strcmp(units, "percentage") || !strcmp(units, "percent") || !strcmp(units, "pcent")) return "%";
	return units;
}

static inline const char *color_map(const char *color) {
	// colors from:
	// https://github.com/badges/shields/blob/master/colorscheme.json
	     if(!strcmp(color, "brightgreen")) return "#4c1";
	else if(!strcmp(color, "green"))       return "#97CA00";
	else if(!strcmp(color, "yellow"))      return "#dfb317";
	else if(!strcmp(color, "yellowgreen")) return "#a4a61d";
	else if(!strcmp(color, "orange"))      return "#fe7d37";
	else if(!strcmp(color, "red"))         return "#e05d44";
	else if(!strcmp(color, "blue"))        return "#007ec6";
	else if(!strcmp(color, "grey"))        return "#555";
	else if(!strcmp(color, "gray"))        return "#555";
	else if(!strcmp(color, "lightgrey"))   return "#9f9f9f";
	else if(!strcmp(color, "lightgray"))   return "#9f9f9f";
	return color;
}

static inline void calc_colorz(const char *color, char *final, size_t len, calculated_number value, int value_is_null) {
	char color_buffer[256 + 1] = "";
	char value_buffer[256 + 1] = "";
	char comparison = '>';

	// example input:
	// color<max|color>min|color:null...

	const char *c = color;
	while(*c) {
		char *dc = color_buffer, *dv = NULL;
		size_t ci = 0, vi = 0;

		const char *t = c;

		while(*t && *t != '|') {
			switch(*t) {
				case ':':
					comparison = '=';
					dv = value_buffer;
					break;

				case '}':
				case ')':
				case '>':
					if(t[1] == '=') {
						comparison = ')';
						t++;
					}
					else
						comparison = '>';
					dv = value_buffer;
					break;

				case '{':
				case '(':
				case '<':
					if(t[1] == '=') {
						comparison = '(';
						t++;
					}
					else
						comparison = '<';
					dv = value_buffer;
					break;

				default:
					if(dv) {
						if(vi < 256) {
							vi++;
							*dv++ = *t;
						}
					}
					else {
						if(ci < 256) {
							ci++;
							*dc++ = *t;
						}
					}
					break;
			}

			t++;
		}

		// prepare for next iteration
		if(*t == '|') t++;
		c = t;

		// do the math
		*dc = '\0';
		if(dv) {
			*dv = '\0';

			if(value_is_null) {
				if(!*value_buffer || !strcmp(value_buffer, "null"))
					break;
			}
			else {
				calculated_number v = strtold(value_buffer, NULL);

				     if(comparison == '<' && value < v) break;
				else if(comparison == '(' && value <= v) break;
				else if(comparison == '>' && value > v) break;
				else if(comparison == ')' && value >= v) break;
				else if(comparison == '=' && value == v) break;
			}
		}
		else
			break;
	}

	const char *b;
	if(color_buffer[0])
		b = color_buffer;
	else
		b = color;

	strncpyz(final, b, len);
}

void buffer_svg(BUFFER *wb, const char *label, calculated_number value, const char *units, const char *label_color, const char *value_color, int value_is_null, int precision) {
	char label_buffer[256 + 1], value_string[512 + 1], value_color_buffer[256 + 1];
	char label_escaped[256 + 1], value_escaped[512 + 1], label_color_escaped[256 + 1], value_color_escaped[256 + 1];
	int label_width, value_width, total_width;

	if(!label_color || !*label_color) label_color = "#555";
	if(!value_color || !*value_color) value_color = (value_is_null)?"#999":"#4c1";

	units = fix_units(units);
	calc_colorz(value_color, value_color_buffer, 256, value, value_is_null);

	char *separator = "";
	if(isalnum(*units)) separator = " ";

	if(value_is_null)
		strcpy(value_string, "-");
	else if(precision < 0) {
		calculated_number abs = (value < (calculated_number)0)?-value:value;
		if(abs > (calculated_number)1000.0)      snprintfz(value_string, 512, "%0.0Lf%s%s", (long double)value, separator, units);
		else if(abs > (calculated_number)100.0)  snprintfz(value_string, 512, "%0.1Lf%s%s", (long double)value, separator, units);
		else if(abs > (calculated_number)1.0)    snprintfz(value_string, 512, "%0.2Lf%s%s", (long double)value, separator, units);
		else if(abs > (calculated_number)0.1)    snprintfz(value_string, 512, "%0.3Lf%s%s", (long double)value, separator, units);
		else                                     snprintfz(value_string, 512, "%0.4Lf%s%s", (long double)value, separator, units);
	}
	else {
		if(precision > 50) precision = 50;
		snprintfz(value_string, 512, "%0.*Lf%s%s", precision, (long double)value, separator, units);
	}

	// we need to copy the label, since verdana11_width may write to it
	strncpyz(label_buffer, label, 256);

	label_width = verdana11_width(label_buffer) + (BADGE_HORIZONTAL_PADDING * 2);
	value_width = verdana11_width(value_string) + (BADGE_HORIZONTAL_PADDING * 2);
	total_width = label_width + value_width;

	escape_xmlz(label_escaped, label_buffer, 256);
	escape_xmlz(value_escaped, value_string, 256);
	escape_xmlz(label_color_escaped, color_map(label_color), 256);
	escape_xmlz(value_color_escaped, color_map(value_color_buffer), 256);

	wb->contenttype = CT_IMAGE_SVG_XML;

	// svg template from:
	// https://raw.githubusercontent.com/badges/shields/master/templates/flat-template.svg
	buffer_sprintf(wb,
		"<svg xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\" width=\"%zu\" height=\"20\">"
			"<linearGradient id=\"smooth\" x2=\"0\" y2=\"100%%\">"
				"<stop offset=\"0\" stop-color=\"#bbb\" stop-opacity=\".1\"/>"
				"<stop offset=\"1\" stop-opacity=\".1\"/>"
			"</linearGradient>"
			"<mask id=\"round\">"
				"<rect width=\"%zu\" height=\"20\" rx=\"3\" fill=\"#fff\"/>"
			"</mask>"
			"<g mask=\"url(#round)\">"
				"<rect width=\"%zu\" height=\"20\" fill=\"%s\"/>"
				"<rect x=\"%zu\" width=\"%zu\" height=\"20\" fill=\"%s\"/>"
				"<rect width=\"%zu\" height=\"20\" fill=\"url(#smooth)\"/>"
			"</g>"
			"<g fill=\"#fff\" text-anchor=\"middle\" font-family=\"DejaVu Sans,Verdana,Geneva,sans-serif\" font-size=\"11\">"
				"<text x=\"%zu\" y=\"15\" fill=\"#010101\" fill-opacity=\".3\">%s</text>"
				"<text x=\"%zu\" y=\"14\">%s</text>"
				"<text x=\"%zu\" y=\"15\" fill=\"#010101\" fill-opacity=\".3\">%s</text>"
				"<text x=\"%zu\" y=\"14\">%s</text>"
			"</g>"
		"</svg>",
		total_width, total_width,
		label_width, label_color_escaped,
		label_width, value_width, value_color_escaped,
		total_width,
		label_width / 2, label_escaped,
		label_width / 2, label_escaped,
		label_width + value_width / 2 -1, value_escaped,
		label_width + value_width / 2 -1, value_escaped);
}
