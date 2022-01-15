// SPDX-License-Identifier: GPL-3.0-or-later

#include "SpearmansRank.h"

/* Check if all values in either vectors are zero */
int SpearmansRank::checkNonZeroVector(std::vector<float> & X) {
	int nonZeroVector = 0;
	
	for(std::vector<float>::size_type i = 0; i != X.size(); i++) {
    	if(X[i] != 0) {
			nonZeroVector = 1;
			break; 
		}
	}
	
	return nonZeroVector;
} 

/* Provide the rank vector of the set of given values */
std::vector<float> SpearmansRank::getRanks(std::vector<float> & X) {

	int N = X.size();

	/* Rank Vector */
	std::vector<float> Rank_X(N);
	
	for(int i = 0; i < N; i++) {
		int r = 1, s = 1;
		
		/* Count no of smaller elements in 0 to i-1 */
		for(int j = 0; j < i; j++) {
			if(X[j] < X[i]) {
				r++;
			}
			if(X[j] == X[i]) {
				s++;
			}
		}
	
		/* Count no of smaller elements in i+1 to N-1 */
		for (int j = i+1; j < N; j++) {
			if(X[j] < X[i]) {
				r++;
			}
			if(X[j] == X[i]) {
				s++;
			}
		}

		/* Use Fractional Rank formula: fractional_rank = r + (n-1)/2 */
		Rank_X[i] = r + (s-1) * 0.5;	
	}
	
	return Rank_X;
}

/* Provide the Pearson correlation coefficient between the given rank values */
float SpearmansRank::getCorrelationCoefficient() {
	float ret = 0;
	int n = rank_x.size();
	float sum_X = 0, sum_Y = 0,	sum_XY = 0;
	float squareSum_X = 0, squareSum_Y = 0;

	/* Do not calculate, if at least one of the vectors is all zeros */
	if(notCorrelated == 0) {
		for (int i = 0; i < n; i++)	{
			/* sum of elements of array X */
			sum_X = sum_X + rank_x[i];

			/* sum of elements of array Y */
			sum_Y = sum_Y + rank_y[i];

			/* sum of X[i] * Y[i] */
			sum_XY = sum_XY + rank_x[i] * rank_y[i];

			/* sum of square of array elements */
			squareSum_X = squareSum_X +	rank_x[i] * rank_x[i];
			squareSum_Y = squareSum_Y +	rank_y[i] * rank_y[i];
		}

		/* The formula for calculating correlation coefficient */
		float corr = (float)(n * sum_XY - sum_X * sum_Y) / sqrt((n * squareSum_X - sum_X * sum_X) *	(n * squareSum_Y - sum_Y * sum_Y));
		ret = corr;
	}
	
	return ret;
}

