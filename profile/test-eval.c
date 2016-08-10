
/*
 * 1. build netdata (as normally)
 * 2. cd profile/
 * 3. compile with:
 *    gcc -O1 -ggdb -Wall -Wextra -I ../src/ -I ../ -o test-eval test-eval.c ../src/log.o ../src/eval.o ../src/common.o -pthread
 *
 */

#include "common.h"

void netdata_cleanup_and_exit(int ret) { exit(ret); }

void indent(int level, int show) {
	int i = level;
	while(i--) printf(" |  ");
	if(show) printf(" \\_ ");
	else printf(" \\_  ");
}

void print_operand(EVAL_OPERAND *op, int level);

void print_value(EVAL_VALUE *v, int level) {
	indent(level, 0);

	switch(v->type) {
		case EVAL_VALUE_INVALID:
			printf("VALUE (NOP)\n");
			break;

		case EVAL_VALUE_NUMBER:
			printf("VALUE %Lf (NUMBER)\n", v->number);
			break;

		case EVAL_VALUE_EXPRESSION:
			printf("VALUE (SUB-EXPRESSION)\n");
			print_operand(v->expression, level+1);
			break;

		default:
			printf("VALUE (INVALID type %d)\n", v->type);
			break;

	}
}

void print_operand(EVAL_OPERAND *op, int level) {

//	if(op->operator != EVAL_OPERATOR_NOP) {
		indent(level, 1);
		if(op->operator) printf("%c (OPERATOR %d, prec: %d)\n", op->operator, op->id, op->precedence);
		else printf("NOP (OPERATOR %d, prec: %d)\n", op->id, op->precedence);
//	}

	int i = op->count;
	while(i--) print_value(&op->ops[i], level + 1);
}

calculated_number evaluate(EVAL_OPERAND *op);

calculated_number evaluate_value(EVAL_VALUE *v) {
	switch(v->type) {
		case EVAL_VALUE_NUMBER:
			return v->number;

		case EVAL_VALUE_EXPRESSION:
			return evaluate(v->expression);

		default:
			fatal("I don't know how to handle EVAL_VALUE type %d", v->type);
	}
}

calculated_number evaluate(EVAL_OPERAND *op) {
	calculated_number n1, n2, r;

	switch(op->operator) {
		case EVAL_OPERATOR_SIGN_PLUS:
			r = evaluate_value(&op->ops[0]);
			break;

		case EVAL_OPERATOR_SIGN_MINUS:
			r = -evaluate_value(&op->ops[0]);
			break;

		case EVAL_OPERATOR_PLUS:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0]);
			n2 = evaluate_value(&op->ops[1]);
			r = n1 + n2;
			printf("%Lf = %Lf %c %Lf\n", r, n1, op->operator, n2);
			break;

		case EVAL_OPERATOR_MINUS:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0]);
			n2 = evaluate_value(&op->ops[1]);
			r = n1 - n2;
			printf("%Lf = %Lf %c %Lf\n", r, n1, op->operator, n2);
			break;

		case EVAL_OPERATOR_MULTIPLY:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0]);
			n2 = evaluate_value(&op->ops[1]);
			r = n1 * n2;
			printf("%Lf = %Lf %c %Lf\n", r, n1, op->operator, n2);
			break;

		case EVAL_OPERATOR_DIVIDE:
			if(op->count != 2)
				fatal("Operator '%c' requires 2 values, but we have %d", op->operator, op->count);
			n1 = evaluate_value(&op->ops[0]);
			n2 = evaluate_value(&op->ops[1]);
			r = n1 / n2;
			printf("%Lf = %Lf %c %Lf\n", r, n1, op->operator, n2);
			break;

		case EVAL_OPERATOR_EXPRESSION_OPEN:
			printf("BEGIN SUB-EXPRESSION\n");
			r = evaluate_value(&op->ops[0]);
			printf("END SUB-EXPRESSION\n");
			break;

		case EVAL_OPERATOR_NOP:
		case EVAL_OPERATOR_VALUE:
			r = evaluate_value(&op->ops[0]);
			break;

		default:
			fatal("I don't know how to handle operator '%c'", op->operator);
			break;
	}

	return r;
}

void print_expression(EVAL_OPERAND *op, const char *failed_at, int error) {
	if(op) {
		printf("<expression root>\n");
		print_operand(op, 0);
		evaluate(op);
		free_expression(op);
	}
	else {
		printf("error: %d, failed_at: '%s'\n", error, (failed_at)?failed_at:"<NONE>");
	}
}


int main(int argc, char **argv) {
	if(argc != 2) {
		fprintf(stderr, "I need an epxression\n");
		exit(1);
	}

	const char *failed_at = NULL;
	int error;
	EVAL_OPERAND *op = parse_expression(argv[1], &failed_at, &error);
	print_expression(op, failed_at, error);
	return 0;
}
