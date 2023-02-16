import {createApp, nextTick} from 'vue';
import $ from 'jquery';
import {vueDelimiters} from './VueComponentLoader.js';

export function initRepoBranchTagDropdown(selector) {
  $(selector).each(function () {
    const $dropdown = $(this);
    const $data = $dropdown.find('.data');
    const data = {
      items: [],
      mode: $data.data('mode'),
      searchTerm: '',
      refName: '',
      noResults: '',
      canCreateBranch: false,
      menuVisible: false,
      createTag: false,
      isViewTag: false,
      isViewBranch: false,
      isViewTree: false,
      active: 0,
      branchForm: '',
      branchURLPrefix: '',
      branchURLSuffix: '',
      tagURLPrefix: '',
      tagURLSuffix: '',
      setAction: false,
      submitForm: false,
    };
    $data.find('.item').each(function () {
      data.items.push({
        name: $(this).text(),
        url: $(this).data('url'),
        branch: $(this).hasClass('branch'),
        tag: $(this).hasClass('tag'),
        selected: $(this).hasClass('selected')
      });
    });
    $data.remove();

    // eslint-disable-next-line unicorn/no-this-assignment
    const elRoot = this;
    const view = createApp({
      delimiters: vueDelimiters,
      data() {
        return data;
      },
      computed: {
        filteredItems() {
          const items = this.items.filter((item) => {
            return ((this.mode === 'branches' && item.branch) || (this.mode === 'tags' && item.tag)) &&
              (!this.searchTerm || item.name.toLowerCase().includes(this.searchTerm.toLowerCase()));
          });

          // no idea how to fix this so linting rule is disabled instead
          this.active = (items.length === 0 && this.showCreateNewBranch ? 0 : -1); // eslint-disable-line vue/no-side-effects-in-computed-properties
          return items;
        },
        showNoResults() {
          return this.filteredItems.length === 0 && !this.showCreateNewBranch;
        },
        showCreateNewBranch() {
          if (!this.canCreateBranch || !this.searchTerm) {
            return false;
          }

          return this.items.filter((item) => item.name.toLowerCase() === this.searchTerm.toLowerCase()).length === 0;
        }
      },

      watch: {
        menuVisible(visible) {
          if (visible) {
            this.focusSearchField();
          }
        }
      },

      beforeMount() {
        this.noResults = elRoot.getAttribute('data-no-results');
        this.canCreateBranch = elRoot.getAttribute('data-can-create-branch') === 'true';
        this.branchForm = elRoot.getAttribute('data-branch-form');
        switch (elRoot.getAttribute('data-view-type')) {
          case 'tree':
            this.isViewTree = true;
            break;
          case 'tag':
            this.isViewTag = true;
            break;
          default:
            this.isViewBranch = true;
            break;
        }
        this.refName = elRoot.getAttribute('data-ref-name');
        this.branchURLPrefix = elRoot.getAttribute('data-branch-url-prefix');
        this.branchURLSuffix = elRoot.getAttribute('data-branch-url-suffix');
        this.tagURLPrefix = elRoot.getAttribute('data-tag-url-prefix');
        this.tagURLSuffix = elRoot.getAttribute('data-tag-url-suffix');
        this.setAction = elRoot.getAttribute('data-set-action') === 'true';
        this.submitForm = elRoot.getAttribute('data-submit-form') === 'true';


        document.body.addEventListener('click', (event) => {
          if (elRoot.contains(event.target)) return;
          if (this.menuVisible) {
            this.menuVisible = false;
          }
        });
      },

      methods: {
        selectItem(item) {
          const prev = this.getSelected();
          if (prev !== null) {
            prev.selected = false;
          }
          item.selected = true;
          const url = (item.tag) ? this.tagURLPrefix + item.url + this.tagURLSuffix : this.branchURLPrefix + item.url + this.branchURLSuffix;
          if (this.branchForm === '') {
            window.location.href = url;
          } else {
            this.isViewTree = false;
            this.isViewTag = false;
            this.isViewBranch = false;
            this.$refs.dropdownRefName.textContent = item.name;
            if (this.setAction) {
              $(`#${this.branchForm}`).attr('action', url);
            } else {
              $(`#${this.branchForm} input[name="refURL"]`).val(url);
            }
            $(`#${this.branchForm} input[name="ref"]`).val(item.name);
            if (item.tag) {
              this.isViewTag = true;
              $(`#${this.branchForm} input[name="refType"]`).val('tag');
            } else {
              this.isViewBranch = true;
              $(`#${this.branchForm} input[name="refType"]`).val('branch');
            }
            if (this.submitForm) {
              $(`#${this.branchForm}`).trigger('submit');
            }
            this.menuVisible = false;
          }
        },
        createNewBranch() {
          if (!this.showCreateNewBranch) return;
          $(this.$refs.newBranchForm).trigger('submit');
        },
        focusSearchField() {
          nextTick(() => {
            this.$refs.searchField.focus();
          });
        },
        getSelected() {
          for (let i = 0, j = this.items.length; i < j; ++i) {
            if (this.items[i].selected) return this.items[i];
          }
          return null;
        },
        getSelectedIndexInFiltered() {
          for (let i = 0, j = this.filteredItems.length; i < j; ++i) {
            if (this.filteredItems[i].selected) return i;
          }
          return -1;
        },
        scrollToActive() {
          let el = this.$refs[`listItem${this.active}`];
          if (!el || !el.length) return;
          if (Array.isArray(el)) {
            el = el[0];
          }

          const cont = this.$refs.scrollContainer;
          if (el.offsetTop < cont.scrollTop) {
            cont.scrollTop = el.offsetTop;
          } else if (el.offsetTop + el.clientHeight > cont.scrollTop + cont.clientHeight) {
            cont.scrollTop = el.offsetTop + el.clientHeight - cont.clientHeight;
          }
        },
        keydown(event) {
          if (event.keyCode === 40) { // arrow down
            event.preventDefault();

            if (this.active === -1) {
              this.active = this.getSelectedIndexInFiltered();
            }

            if (this.active + (this.showCreateNewBranch ? 0 : 1) >= this.filteredItems.length) {
              return;
            }
            this.active++;
            this.scrollToActive();
          } else if (event.keyCode === 38) { // arrow up
            event.preventDefault();

            if (this.active === -1) {
              this.active = this.getSelectedIndexInFiltered();
            }

            if (this.active <= 0) {
              return;
            }
            this.active--;
            this.scrollToActive();
          } else if (event.keyCode === 13) { // enter
            event.preventDefault();

            if (this.active >= this.filteredItems.length) {
              this.createNewBranch();
            } else if (this.active >= 0) {
              this.selectItem(this.filteredItems[this.active]);
            }
          } else if (event.keyCode === 27) { // escape
            event.preventDefault();
            this.menuVisible = false;
          }
        }
      }
    });
    view.mount(this);
  });
}
