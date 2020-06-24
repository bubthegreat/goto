package parser

import (
	"fmt"
	"strconv"

	"goto/ast"
	"goto/lexer"
	"goto/token"
)

const ( // These represent the operator precedence values.
	_int = iota
	LOWEST
	EQUALS      // ==
	LESSGREATER // > or <
	PLUS        // +
	MULTIPLY    // *
	PREFIX      // -X or !X
	CALL        // myFunction(X)
)

var precedences = map[token.Type]int{
	token.EQ:       EQUALS,
	token.NOT_EQ:   EQUALS,
	token.LT:       LESSGREATER,
	token.GT:       LESSGREATER,
	token.LT_EQ:    LESSGREATER,
	token.GT_EQ:    LESSGREATER,
	token.PLUS:     PLUS,
	token.MINUS:    PLUS,
	token.DIVIDE:   MULTIPLY,
	token.MULTIPLY: MULTIPLY,
	token.LPAREN:   CALL,
}

type (
	prefixParsefn func() ast.Expression
	infixParsefn  func(ast.Expression) ast.Expression
)

type Parser struct {
	l *lexer.Lexer

	currToken token.Token
	peekToken token.Token

	errors []string

	prefixParsefns map[token.Type]prefixParsefn
	infixParsefns  map[token.Type]infixParsefn
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []string{},
	}

	p.prefixParsefns = make(map[token.Type]prefixParsefn)
	prefixfns := []struct {
		token   token.Type
		parsefn prefixParsefn
	}{
		{token.IDENT, p.parseIdentifier},
		{token.INT, p.parseIntegerLiteral},
		{token.NOT, p.parsePrefixExpression},
		{token.MINUS, p.parsePrefixExpression},
		{token.TRUE, p.parseBoolean},
		{token.FALSE, p.parseBoolean},
		{token.STRING, p.parseString},
		{token.LPAREN, p.parseGroupedExpression},
	}

	for _, fn := range prefixfns {
		p.registerPrefix(fn.token, fn.parsefn)
	}

	p.infixParsefns = make(map[token.Type]infixParsefn)
	for keys := range precedences {
		p.registerInfix(keys, p.parseInfixExpression)
	}

	p.registerInfix(token.LPAREN, p.parseCallExpression)

	p.setToken() // Only to be called for initialization of Parser pointers

	return p
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) nextToken() {
	p.currToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) setToken() {
	p.currToken = p.l.NextToken()
	p.peekToken = p.l.NextToken()
}

func (p *Parser) currTokenIs(t token.Type) bool {
	return p.currToken.Type == t
}

func (p *Parser) peekTokenIs(t token.Type) bool {
	return p.peekToken.Type == t
}

func (p *Parser) peekError(t token.Type) {
	msg := fmt.Sprintf("expected next token to be %s , got %s instead", t, p.peekToken.Type)
	p.errors = append(p.errors, msg)

}

func (p *Parser) expectPeek(t token.Type) bool {

	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}

	p.peekError(t)
	return false
}

func (p *Parser) registerPrefix(Type token.Type, fn prefixParsefn) {
	p.prefixParsefns[Type] = fn
}

func (p *Parser) registerInfix(Type token.Type, fn infixParsefn) {
	p.infixParsefns[Type] = fn
}

func (p *Parser) parseBoolean() ast.Expression {
	return &ast.Boolean{Token: p.currToken, Value: p.currTokenIs(token.TRUE)}
}

func (p *Parser) parseString() ast.Expression {
	return &ast.String{Token: p.currToken, Value: p.currToken.Literal}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.currToken}

	value, err := strconv.ParseInt(p.currToken.Literal, 0, 64)

	if err != nil {
		msg := fmt.Sprintf("could not parse %q as integer", p.currToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}

	lit.Value = value

	return lit
}

func (p *Parser) parseIdentifier() ast.Expression {
	return &ast.Identifier{Token: p.currToken, Value: p.currToken.Literal}
}

func (p *Parser) noPrefixParseFnError(t token.Type) {
	msg := fmt.Sprintf("no prefix parse function for %s found", t)
	p.errors = append(p.errors, msg)
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	prefixexp := &ast.PrefixExpression{Token: p.currToken, Operator: p.currToken.Literal}

	p.nextToken()

	prefixexp.Right = p.parseExpression(PREFIX)

	return prefixexp
}

func (p *Parser) currPrecedence() int {
	if p, ok := precedences[p.currToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	infixexp := &ast.InfixExpression{
		Token:    p.currToken,
		Operator: p.currToken.Literal,
		Left:     left,
	}

	precedence := p.currPrecedence()
	p.nextToken()

	infixexp.Right = p.parseExpression(precedence)
	return infixexp
}

func (p *Parser) parseCallArguments() *ast.ExpressionList {
	args := &ast.ExpressionList{Token: p.currToken}

	p.nextToken()

	for !p.currTokenIs(token.RPAREN) && !p.currTokenIs(token.EOF) {
		exp := p.parseExpression(LOWEST)
		args.Expressions = append(args.Expressions, &exp)

		if p.peekTokenIs(token.COMMA) {
			p.nextToken() // TODO: add a utility to do multiple token jumps
			p.nextToken()
			continue
		}
		if p.peekTokenIs(token.RPAREN) {
			p.nextToken()
			break
		}
		// TODO: error message
		return nil
	}

	return args
}

func (p *Parser) parseCallExpression(left ast.Expression) ast.Expression {
	exp := &ast.CallExpression{Token: p.currToken}
	fname, ok := left.(*ast.Identifier)
	if !ok {
		return nil
	}
	exp.FunctionName = fname
	exp.ArgumentList = p.parseCallArguments()
	return exp
}

func (p *Parser) parseExpression(precedence int) ast.Expression { // returns expression on the same or higher precedence level
	prefix := p.prefixParsefns[p.currToken.Type]

	if prefix == nil {
		p.noPrefixParseFnError(p.currToken.Type)
		return nil
	}

	leftExp := prefix()

	for !p.peekTokenIs(token.SEMI) && precedence < p.peekPrecedence() {
		infix := p.infixParsefns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}

		p.nextToken()

		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) parseGroupedExpression() ast.Expression {
	p.nextToken()

	exp := p.parseExpression(LOWEST)

	if p.expectPeek(token.RPAREN) {
		return exp
	}

	return nil
}

func (p *Parser) parseVarStatement() *ast.VarStatement {
	stmt := &ast.VarStatement{Token: p.currToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.currToken, Value: p.currToken.Literal}

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}

	p.nextToken()

	stmt.Value = p.parseExpression(LOWEST)

	if !p.expectPeek(token.SEMI) {
		return nil
	}

	return stmt
}

func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	stmt := &ast.ReturnStatement{Token: p.currToken}

	p.nextToken()

	stmt.ReturnValue = p.parseExpression(LOWEST)

	if !p.expectPeek(token.SEMI) {
		return nil
	}

	return stmt
}

func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	block := &ast.BlockStatement{Token: p.currToken}
	p.nextToken()
	for !p.currTokenIs(token.RBRACE) && !p.currTokenIs(token.EOF) {

		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()

	}

	return block
}

func (p *Parser) parseIfStatement() *ast.IfStatement {
	stmt := &ast.IfStatement{Token: p.currToken}

	p.nextToken()

	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Consequence = p.parseBlockStatement()

	if !p.peekTokenIs(token.ELSE) {
		return stmt
	}

	p.nextToken()

	if p.peekTokenIs(token.IF) {
		p.nextToken()
		stmt.FollowIf = p.parseIfStatement()
	} else if !p.expectPeek(token.LBRACE) {
		return nil
	} else {
		stmt.Alternative = p.parseBlockStatement()
	}

	return stmt
}

func (p *Parser) parseIdentifierList() *ast.IdentifierList {
	identlist := &ast.IdentifierList{Token: p.currToken}

	p.nextToken()

	for !p.currTokenIs(token.RPAREN) && !p.currTokenIs(token.EOF) {

		ident, ok := p.parseIdentifier().(*ast.Identifier)

		if !ok {
			// TODO: Error message
			return nil
		}

		if ident != nil {
			identlist.Identifiers = append(identlist.Identifiers, ident)
		}
		if p.peekTokenIs(token.COMMA) {
			p.nextToken() // TODO: add a utility to do multiple token jumps
			p.nextToken()
			continue
		}
		if p.peekTokenIs(token.RPAREN) {
			p.nextToken()
			break
		}
		// TODO: error message
		return nil
	}

	return identlist
}

func (p *Parser) parseFuncStatement() *ast.FuncStatement {
	stmt := &ast.FuncStatement{Token: p.currToken}

	p.nextToken()

	name, ok := p.parseIdentifier().(*ast.Identifier)

	if !ok {
		//TODO: Error message
		return nil
	}

	stmt.Name = name

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	stmt.ParameterList = p.parseIdentifierList()

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.FuncBody = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	stmt := &ast.ExpressionStatement{Token: p.currToken}

	stmt.Expression = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMI) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.currToken.Type {
	case token.VAR:
		return p.parseVarStatement()
	case token.RETURN:
		return p.parseReturnStatement()
	case token.IF:
		return p.parseIfStatement()
	case token.LBRACE:
		return p.parseBlockStatement()
	case token.FUNC:
		return p.parseFuncStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{}
	program.Statements = []ast.Statement{}

	for p.currToken.Type != token.EOF {
		stmt := p.parseStatement()

		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}

		p.nextToken()
	}

	return program
}
