// SPDX-License-Identifier: GPL-3.0-or-later

'use strict';

var path = require('path');
var fs = require('fs');
var PDFDocument = require('pdfkit');
var doc = new PDFDocument({size:'A4', layout:'landscape'});

function loadFont(fontPaths, callback) {
    for (let fontPath of fontPaths) {
        try {
            doc = doc.font(fontPath);
            if (callback) { callback(null); }
            return; // Exit once a font is loaded successfully
        } catch(err) {
            // Log error but continue to next font path
            console.error(`Failed to load font from path: ${fontPath}. Error: ${err.message}`);
        }
    }

    // If we reached here, none of the fonts were loaded successfully.
    console.error('All font paths failed. Stopping execution.');
    process.exit(1); // Exit with an error code
}

loadFont(['IBMPlexSans-Bold.ttf'], function(err) {
    if (err) {
        console.error('Could not load any of the specified fonts.');
    }
});

doc = doc.fontSize(250);

function measureCombination(charA, charB) {
    return doc.widthOfString(charA + charB);
}

function getCharRepresentation(charCode) {
    return (charCode >= 32 && charCode <= 126) ? String.fromCharCode(charCode) : '';
}

function generateCombinationArray() {
    let output = "static const unsigned short int ibm_plex_sans_bold_250[128][128] = {\n";

    for (let i = 0; i <= 126; i++) {
        output += "    {";  // Start of inner array
        for (let j = 0; j <= 126; j++) {
            let charA = getCharRepresentation(i);
            let charB = getCharRepresentation(j);
            let width = measureCombination(charA, charB) - doc.widthOfString(charB);
            let encodedWidth = Math.round(width * 100);  // Multiply by 100 and round

            if(charA === '*' && charB == '/')
                charB = '\\/';

            if(charA === '/' && charB == '*')
                charB = '\\*';

            output += `${encodedWidth} /* ${charA}${charB} */`;
            if (j < 126) {
                output += ", ";
            }
        }
        output += "},\n";  // End of inner array
    }
    output += "};\n";  // End of 2D array

    return output;
}

console.log(generateCombinationArray());
console.log('static const unsigned short int ibm_plex_sans_bold_250_em_size = ' + Math.round(doc.widthOfString('M') * 100) + ';');
