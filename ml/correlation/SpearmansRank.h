// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SPEARMANSRANK_H
#define SPEARMANSRANK_H

#include <vector>
#include <cmath>

class SpearmansRank {
public:
    SpearmansRank(std::vector<float> X, std::vector<float> Y) {
        if((checkNonZeroVector(X) == 1) && (checkNonZeroVector(Y) == 1)) {
            rank_x = getRanks(X);
            rank_y = getRanks(Y);
        }
        else {
            notCorrelated = 1;
        }
    };

    float getCorrelationCoefficient();
    
private:
    int checkNonZeroVector(std::vector<float> & X);
    std::vector<float> getRanks(std::vector<float> & X);    
    
    std::vector<float> rank_x;
    std::vector<float> rank_y;

    int notCorrelated = 0;
};

#endif /* SPEARMANSRANK_H */