/* SPDX-License-Identifier: GPL-3.0-or-later */

/*
 * 1. build netdata (as normally)
 * 2. cd profile/
 * 3. compile with:
 *    gcc -O1 -ggdb -Wall -Wextra -I ../src/ -I ../ -o test-eval test-eval.c ../src/log.o ../src/eval.o ../src/common.o ../src/clocks.o ../src/web_buffer.o ../src/storage_number.o -pthread -lm
 */

#include "config.h"
#include "common.h"
#include "clocks.h"

void netdata_cleanup_and_exit(int ret) { exit(ret); }

/*
void indent(int level, int show) {
	int i = level;
	while(i--) printf(" |  ");
	if(show) printf(" \\_ ");
	else printf(" \\_  ");
}

void print_node(EVAL_NODE *op, int level);

void print_value(EVAL_VALUE *v, int level) {
	indent(level, 0);

	switch(v->type) {
		case EVAL_VALUE_INVALID:
			printf("value (NOP)\n");
			break;

		case EVAL_VALUE_NUMBER:
			printf("value %Lf (NUMBER)\n", v->number);
			break;

		case EVAL_VALUE_EXPRESSION:
			printf("value (SUB-EXPRESSION)\n");
			print_node(v->expression, level+1);
			break;

		default:
			printf("value (INVALID type %d)\n", v->type);
			break;

	}
}

void print_node(EVAL_NODE *op, int level) {

//	if(op->operator != EVAL_OPERATOR_NOP) {
		indent(level, 1);
		if(op->operator) printf("%c (node %d, precedence: %d)\n", op->operator, op->id, op->precedence);
		else printf("NOP (node %d, precedence: %d)\n", op->id, op->precedence);
//	}

	int i = op->count;
	while(i--) print_value(&op->ops[i], level + 1);
}

calculated_number evaluate(EVAL_NODE *op, int depth);

calculated_number evaluate_value(EVAL_VALUE *v, int depth) {
	switch(v->type) {
		case EVAL_VALUE_NUMBER:
			return v->number;

		case EVAL_VALUE_EXPRESSION:
			return evaluate(v->expression, depth);

		default:
			fatal("I don't know how to handle EVAL_VALUE type %d", v->type);
	}
}

void print_depth(int depth) {
	static int count = 0;

	printf("%d. ", ++count);
	while(depth--) printf("    ");
}

calculated_number evaluate(EVAL_NODE *op, int depth) {
	calculated_number n1, n2, r;

	switch(op->operator) {
		case EVAL_OPERATOR_SIGN_PLUS:
			r = evaluate_value(&op->ops[0], depth);
			break;

		case EVAL_OPERATOR_SIGN_MINUS:
			r = -evaluate_value(&op->ops[0], depth);
			break;

		case EVAL_OPERATOR_PLUS:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 + n2;
			print_depth(depth);
			printf("%Lf = %Lf + %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_MINUS:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 - n2;
			print_depth(depth);
			printf("%Lf = %Lf - %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_MULTIPLY:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 * n2;
			print_depth(depth);
			printf("%Lf = %Lf * %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_DIVIDE:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 / n2;
			print_depth(depth);
			printf("%Lf = %Lf / %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_NOT:
			n1 = evaluate_value(&op->ops[0], depth);
			r = !n1;
			print_depth(depth);
			printf("%Lf = NOT %Lf\n", r, n1);
			break;

		case EVAL_OPERATOR_AND:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 && n2;
			print_depth(depth);
			printf("%Lf = %Lf AND %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_OR:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 || n2;
			print_depth(depth);
			printf("%Lf = %Lf OR %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_GREATER_THAN_OR_EQUAL:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 >= n2;
			print_depth(depth);
			printf("%Lf = %Lf >= %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_LESS_THAN_OR_EQUAL:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 <= n2;
			print_depth(depth);
			printf("%Lf = %Lf <= %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_GREATER:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 > n2;
			print_depth(depth);
			printf("%Lf = %Lf > %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_LESS:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 < n2;
			print_depth(depth);
			printf("%Lf = %Lf < %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_NOT_EQUAL:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 != n2;
			print_depth(depth);
			printf("%Lf = %Lf <> %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_EQUAL:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0], depth);
			n2 = evaluate_value(&op->ops[1], depth);
			r = n1 == n2;
			print_depth(depth);
			printf("%Lf = %Lf == %Lf\n", r, n1, n2);
			break;

		case EVAL_OPERATOR_EXPRESSION_OPEN:
			printf("BEGIN SUB-EXPRESSION\n");
			r = evaluate_value(&op->ops[0], depth + 1);
			printf("END SUB-EXPRESSION\n");
			break;

		case EVAL_OPERATOR_NOP:
		case EVAL_OPERATOR_VALUE:
			r = evaluate_value(&op->ops[0], depth);
			break;

		default:
			error("I don't know how to handle operator '%c'", op->operator);
			r = 0;
			break;
	}

	return r;
}


void print_expression(EVAL_NODE *op, const char *failed_at, int error) {
	if(op) {
		printf("expression tree:\n");
		print_node(op, 0);

		printf("\nevaluation steps:\n");
		evaluate(op, 0);
		
		int error;
		calculated_number ret = expression_evaluate(op, &error);
		printf("\ninternal evaluator:\nSTATUS: %d, RESULT = %Lf\n", error, ret);

		expression_free(op);
	}
	else {
		printf("error: %d, failed_at: '%s'\n", error, (failed_at)?failed_at:"<NONE>");
	}
}
*/

int health_variable_lookup(const char *variable, uint32_t hash, RRDCALC *rc, calculated_number *result) {
	(void)variable;
	(void)hash;
	(void)rc;
	(void)result;

	return 0;
}

int main(int argc, char **argv) {
	if(argc != 2) {
		fprintf(stderr, "I need an epxression (enclose it in single-quotes (') as a single parameter)\n");
		exit(1);
	}

	const char *failed_at = NULL;
	int error;

	EVAL_EXPRESSION *exp = expression_parse(argv[1], &failed_at, &error);
	if(!exp)
		printf("\nPARSING FAILED\nExpression: '%s'\nParsing stopped at: '%s'\nParsing error code: %d (%s)\n", argv[1], (failed_at)?((*failed_at)?failed_at:"<END OF EXPRESSION>"):"<NONE>", error, expression_strerror(error));
	
	else {
		printf("\nPARSING OK\nExpression: '%s'\nParsed as : '%s'\nParsing error code: %d (%s)\n", argv[1], exp->parsed_as, error, expression_strerror(error));

		if(expression_evaluate(exp)) {
			printf("\nEvaluates to: %Lf\n\n", exp->result);
		}
		else {
			printf("\nEvaluation failed with code %d and message: %s\n\n", exp->error, buffer_tostring(exp->error_msg));
		}
		expression_free(exp);
	}

	return 0;
}
