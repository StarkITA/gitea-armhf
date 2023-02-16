import {createApp, nextTick} from 'vue';
import $ from 'jquery';
import {initVueSvg, vueDelimiters} from './VueComponentLoader.js';
import {initTooltip} from '../modules/tippy.js';

const {appSubUrl, assetUrlPrefix, pageData} = window.config;

function initVueComponents(app) {
  app.component('repo-search', {
    delimiters: vueDelimiters,
    props: {
      searchLimit: {
        type: Number,
        default: 10
      },
      subUrl: {
        type: String,
        required: true
      },
      uid: {
        type: Number,
        default: 0
      },
      teamId: {
        type: Number,
        required: false,
        default: 0
      },
      organizations: {
        type: Array,
        default: () => [],
      },
      isOrganization: {
        type: Boolean,
        default: true
      },
      canCreateOrganization: {
        type: Boolean,
        default: false
      },
      organizationsTotalCount: {
        type: Number,
        default: 0
      },
      moreReposLink: {
        type: String,
        default: ''
      }
    },

    data() {
      const params = new URLSearchParams(window.location.search);

      let tab = params.get('repo-search-tab');
      if (!tab) {
        tab = 'repos';
      }

      let reposFilter = params.get('repo-search-filter');
      if (!reposFilter) {
        reposFilter = 'all';
      }

      let privateFilter = params.get('repo-search-private');
      if (!privateFilter) {
        privateFilter = 'both';
      }

      let archivedFilter = params.get('repo-search-archived');
      if (!archivedFilter) {
        archivedFilter = 'unarchived';
      }

      let searchQuery = params.get('repo-search-query');
      if (!searchQuery) {
        searchQuery = '';
      }

      let page = 1;
      try {
        page = parseInt(params.get('repo-search-page'));
      } catch {
        // noop
      }
      if (!page) {
        page = 1;
      }

      return {
        tab,
        repos: [],
        reposTotalCount: 0,
        reposFilter,
        archivedFilter,
        privateFilter,
        page,
        finalPage: 1,
        searchQuery,
        isLoading: false,
        staticPrefix: assetUrlPrefix,
        counts: {},
        repoTypes: {
          all: {
            searchMode: '',
          },
          forks: {
            searchMode: 'fork',
          },
          mirrors: {
            searchMode: 'mirror',
          },
          sources: {
            searchMode: 'source',
          },
          collaborative: {
            searchMode: 'collaborative',
          },
        }
      };
    },

    computed: {
      // used in `repolist.tmpl`
      showMoreReposLink() {
        return this.repos.length > 0 && this.repos.length < this.counts[`${this.reposFilter}:${this.archivedFilter}:${this.privateFilter}`];
      },
      searchURL() {
        return `${this.subUrl}/repo/search?sort=updated&order=desc&uid=${this.uid}&team_id=${this.teamId}&q=${this.searchQuery
        }&page=${this.page}&limit=${this.searchLimit}&mode=${this.repoTypes[this.reposFilter].searchMode
        }${this.reposFilter !== 'all' ? '&exclusive=1' : ''
        }${this.archivedFilter === 'archived' ? '&archived=true' : ''}${this.archivedFilter === 'unarchived' ? '&archived=false' : ''
        }${this.privateFilter === 'private' ? '&is_private=true' : ''}${this.privateFilter === 'public' ? '&is_private=false' : ''
        }`;
      },
      repoTypeCount() {
        return this.counts[`${this.reposFilter}:${this.archivedFilter}:${this.privateFilter}`];
      }
    },

    mounted() {
      const el = document.getElementById('dashboard-repo-list');
      this.changeReposFilter(this.reposFilter);
      for (const elTooltip of el.querySelectorAll('.tooltip')) {
        initTooltip(elTooltip);
      }
      $(el).find('.dropdown').dropdown();
      this.setCheckboxes();
      nextTick(() => {
        this.$refs.search.focus();
      });
    },

    methods: {
      changeTab(t) {
        this.tab = t;
        this.updateHistory();
      },

      setCheckboxes() {
        switch (this.archivedFilter) {
          case 'unarchived':
            $('#archivedFilterCheckbox').checkbox('set unchecked');
            break;
          case 'archived':
            $('#archivedFilterCheckbox').checkbox('set checked');
            break;
          case 'both':
            $('#archivedFilterCheckbox').checkbox('set indeterminate');
            break;
          default:
            this.archivedFilter = 'unarchived';
            $('#archivedFilterCheckbox').checkbox('set unchecked');
            break;
        }
        switch (this.privateFilter) {
          case 'public':
            $('#privateFilterCheckbox').checkbox('set unchecked');
            break;
          case 'private':
            $('#privateFilterCheckbox').checkbox('set checked');
            break;
          case 'both':
            $('#privateFilterCheckbox').checkbox('set indeterminate');
            break;
          default:
            this.privateFilter = 'both';
            $('#privateFilterCheckbox').checkbox('set indeterminate');
            break;
        }
      },

      changeReposFilter(filter) {
        this.reposFilter = filter;
        this.repos = [];
        this.page = 1;
        this.counts[`${filter}:${this.archivedFilter}:${this.privateFilter}`] = 0;
        this.searchRepos();
      },

      updateHistory() {
        const params = new URLSearchParams(window.location.search);

        if (this.tab === 'repos') {
          params.delete('repo-search-tab');
        } else {
          params.set('repo-search-tab', this.tab);
        }

        if (this.reposFilter === 'all') {
          params.delete('repo-search-filter');
        } else {
          params.set('repo-search-filter', this.reposFilter);
        }

        if (this.privateFilter === 'both') {
          params.delete('repo-search-private');
        } else {
          params.set('repo-search-private', this.privateFilter);
        }

        if (this.archivedFilter === 'unarchived') {
          params.delete('repo-search-archived');
        } else {
          params.set('repo-search-archived', this.archivedFilter);
        }

        if (this.searchQuery === '') {
          params.delete('repo-search-query');
        } else {
          params.set('repo-search-query', this.searchQuery);
        }

        if (this.page === 1) {
          params.delete('repo-search-page');
        } else {
          params.set('repo-search-page', `${this.page}`);
        }

        const queryString = params.toString();
        if (queryString) {
          window.history.replaceState({}, '', `?${queryString}`);
        } else {
          window.history.replaceState({}, '', window.location.pathname);
        }
      },

      toggleArchivedFilter() {
        switch (this.archivedFilter) {
          case 'both':
            this.archivedFilter = 'unarchived';
            break;
          case 'unarchived':
            this.archivedFilter = 'archived';
            break;
          case 'archived':
            this.archivedFilter = 'both';
            break;
          default:
            this.archivedFilter = 'unarchived';
            break;
        }
        this.page = 1;
        this.repos = [];
        this.setCheckboxes();
        this.counts[`${this.reposFilter}:${this.archivedFilter}:${this.privateFilter}`] = 0;
        this.searchRepos();
      },

      togglePrivateFilter() {
        switch (this.privateFilter) {
          case 'both':
            this.privateFilter = 'public';
            break;
          case 'public':
            this.privateFilter = 'private';
            break;
          case 'private':
            this.privateFilter = 'both';
            break;
          default:
            this.privateFilter = 'both';
            break;
        }
        this.page = 1;
        this.repos = [];
        this.setCheckboxes();
        this.counts[`${this.reposFilter}:${this.archivedFilter}:${this.privateFilter}`] = 0;
        this.searchRepos();
      },


      changePage(page) {
        this.page = page;
        if (this.page > this.finalPage) {
          this.page = this.finalPage;
        }
        if (this.page < 1) {
          this.page = 1;
        }
        this.repos = [];
        this.counts[`${this.reposFilter}:${this.archivedFilter}:${this.privateFilter}`] = 0;
        this.searchRepos();
      },

      async searchRepos() {
        this.isLoading = true;

        const searchedMode = this.repoTypes[this.reposFilter].searchMode;
        const searchedURL = this.searchURL;
        const searchedQuery = this.searchQuery;

        let response, json;
        try {
          if (!this.reposTotalCount) {
            const totalCountSearchURL = `${this.subUrl}/repo/search?count_only=1&uid=${this.uid}&team_id=${this.teamId}&q=&page=1&mode=`;
            response = await fetch(totalCountSearchURL);
            this.reposTotalCount = response.headers.get('X-Total-Count');
          }

          response = await fetch(searchedURL);
          json = await response.json();
        } catch {
          if (searchedURL === this.searchURL) {
            this.isLoading = false;
          }
          return;
        }

        if (searchedURL === this.searchURL) {
          this.repos = json.data;
          const count = response.headers.get('X-Total-Count');
          if (searchedQuery === '' && searchedMode === '' && this.archivedFilter === 'both') {
            this.reposTotalCount = count;
          }
          this.counts[`${this.reposFilter}:${this.archivedFilter}:${this.privateFilter}`] = count;
          this.finalPage = Math.ceil(count / this.searchLimit);
          this.updateHistory();
          this.isLoading = false;
        }
      },

      repoIcon(repo) {
        if (repo.fork) {
          return 'octicon-repo-forked';
        } else if (repo.mirror) {
          return 'octicon-mirror';
        } else if (repo.template) {
          return `octicon-repo-template`;
        } else if (repo.private) {
          return 'octicon-lock';
        } else if (repo.internal) {
          return 'octicon-repo';
        }
        return 'octicon-repo';
      }
    },

    template: document.getElementById('dashboard-repo-list-template'),
  });
}

export function initDashboardRepoList() {
  const el = document.getElementById('dashboard-repo-list');
  const dashboardRepoListData = pageData.dashboardRepoList || null;
  if (!el || !dashboardRepoListData) return;

  const app = createApp({
    delimiters: vueDelimiters,
    data() {
      return {
        searchLimit: dashboardRepoListData.searchLimit || 0,
        subUrl: appSubUrl,
        uid: dashboardRepoListData.uid || 0,
      };
    },
  });
  initVueSvg(app);
  initVueComponents(app);
  app.mount(el);
}
