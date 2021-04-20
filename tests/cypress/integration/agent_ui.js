/// <reference types="cypress" />

context('Assertions', () => {
  beforeEach(() => {
    cy.visit('localhost:19999')
  })

  describe('Implicit Assertions', () => {
    it('.should() - Assert menu_system text', () => {
        cy.get('h1#menu_system').contains('System Overview');
    })
  })

})
