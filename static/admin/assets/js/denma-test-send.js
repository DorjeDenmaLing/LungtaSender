// A campaign's test e-mail: the addresses typed and a list (cmd/denma_testlist.go).
export function denmaTestSend() {
  return {
    testList: 0,

    // The lists a test can be sent to, by name.
    denmaTestLists() {
      return (window._lists || []).slice().sort((a, b) => a.name.localeCompare(b.name));
    },

    // Whether a test has recipients chosen.
    denmaTestChosen() {
      return this.form.testEmails.length > 0 || this.testList > 0;
    },
  };
}
