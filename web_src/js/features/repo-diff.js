import $ from 'jquery';
import {initCompReactionSelector} from './comp/ReactionSelector.js';
import {initRepoIssueContentHistory} from './repo-issue-content.js';
import {validateTextareaNonEmpty} from './comp/EasyMDE.js';
import {initViewedCheckboxListenerFor, countAndUpdateViewedFiles} from './pull-view-file.js';
import {initTooltip} from '../modules/tippy.js';

const {csrfToken} = window.config;

export function initRepoDiffReviewButton() {
  const $reviewBox = $('#review-box');
  const $counter = $reviewBox.find('.review-comments-counter');

  $(document).on('click', 'button[name="is_review"]', (e) => {
    const $form = $(e.target).closest('form');
    $form.append('<input type="hidden" name="is_review" value="true">');

    // Watch for the form's submit event.
    $form.on('submit', () => {
      const num = parseInt($counter.attr('data-pending-comment-number')) + 1 || 1;
      $counter.attr('data-pending-comment-number', num);
      $counter.text(num);
      // Force the browser to reflow the DOM. This is to ensure that the browser replay the animation
      $reviewBox.removeClass('pulse');
      $reviewBox.width();
      $reviewBox.addClass('pulse');
    });
  });
}

export function initRepoDiffFileViewToggle() {
  $('.file-view-toggle').on('click', function () {
    const $this = $(this);
    $this.parent().children().removeClass('active');
    $this.addClass('active');

    const $target = $($this.data('toggle-selector'));
    $target.parent().children().addClass('hide');
    $target.removeClass('hide');
  });
}

export function initRepoDiffConversationForm() {
  $(document).on('submit', '.conversation-holder form', async (e) => {
    e.preventDefault();

    const $form = $(e.target);
    const $textArea = $form.find('textarea');
    if (!validateTextareaNonEmpty($textArea)) {
      return;
    }

    const formDataString = String(new URLSearchParams(new FormData($form[0])));
    const $newConversationHolder = $(await $.post($form.attr('action'), formDataString));
    const {path, side, idx} = $newConversationHolder.data();

    $newConversationHolder.find('.tooltip').each(function () {
      initTooltip(this);
    });

    $form.closest('.conversation-holder').replaceWith($newConversationHolder);
    if ($form.closest('tr').data('line-type') === 'same') {
      $(`[data-path="${path}"] a.add-code-comment[data-idx="${idx}"]`).addClass('invisible');
    } else {
      $(`[data-path="${path}"] a.add-code-comment[data-side="${side}"][data-idx="${idx}"]`).addClass('invisible');
    }
    $newConversationHolder.find('.dropdown').dropdown();
    initCompReactionSelector($newConversationHolder);
  });

  $(document).on('click', '.resolve-conversation', async function (e) {
    e.preventDefault();
    const comment_id = $(this).data('comment-id');
    const origin = $(this).data('origin');
    const action = $(this).data('action');
    const url = $(this).data('update-url');

    const data = await $.post(url, {_csrf: csrfToken, origin, action, comment_id});

    if ($(this).closest('.conversation-holder').length) {
      const conversation = $(data);
      $(this).closest('.conversation-holder').replaceWith(conversation);
      conversation.find('.dropdown').dropdown();
      initCompReactionSelector(conversation);
    } else {
      window.location.reload();
    }
  });
}

export function initRepoDiffConversationNav() {
  // Previous/Next code review conversation
  $(document).on('click', '.previous-conversation', (e) => {
    const $conversation = $(e.currentTarget).closest('.comment-code-cloud');
    const $conversations = $('.comment-code-cloud:not(.hide)');
    const index = $conversations.index($conversation);
    const previousIndex = index > 0 ? index - 1 : $conversations.length - 1;
    const $previousConversation = $conversations.eq(previousIndex);
    const anchor = $previousConversation.find('.comment').first().attr('id');
    window.location.href = `#${anchor}`;
  });
  $(document).on('click', '.next-conversation', (e) => {
    const $conversation = $(e.currentTarget).closest('.comment-code-cloud');
    const $conversations = $('.comment-code-cloud:not(.hide)');
    const index = $conversations.index($conversation);
    const nextIndex = index < $conversations.length - 1 ? index + 1 : 0;
    const $nextConversation = $conversations.eq(nextIndex);
    const anchor = $nextConversation.find('.comment').first().attr('id');
    window.location.href = `#${anchor}`;
  });
}

// Will be called when the show more (files) button has been pressed
function onShowMoreFiles() {
  initRepoIssueContentHistory();
  initViewedCheckboxListenerFor();
  countAndUpdateViewedFiles();
}

export function doLoadMoreFiles(link, diffEnd, callback) {
  const url = `${link}?skip-to=${diffEnd}&file-only=true`;
  loadMoreFiles(url, callback);
}

function loadMoreFiles(url, callback) {
  const $target = $('a#diff-show-more-files');
  if ($target.hasClass('disabled')) {
    callback();
    return;
  }
  $target.addClass('disabled');
  $.ajax({
    type: 'GET',
    url,
  }).done((resp) => {
    if (!resp) {
      $target.removeClass('disabled');
      callback(resp);
      return;
    }
    $('#diff-incomplete').replaceWith($(resp).find('#diff-file-boxes').children());
    // By simply rerunning the script we add the new data to our existing
    // pagedata object. this triggers vue and the filetree and filelist will
    // render the new elements.
    $('body').append($(resp).find('script#diff-data-script'));
    onShowMoreFiles();
    callback(resp);
  }).fail(() => {
    $target.removeClass('disabled');
    callback();
  });
}

export function initRepoDiffShowMore() {
  $(document).on('click', 'a#diff-show-more-files', (e) => {
    e.preventDefault();

    const $target = $(e.target);
    loadMoreFiles($target.data('href'), () => {});
  });

  $(document).on('click', 'a.diff-load-button', (e) => {
    e.preventDefault();
    const $target = $(e.target);

    if ($target.hasClass('disabled')) {
      return;
    }

    $target.addClass('disabled');

    const url = $target.data('href');
    $.ajax({
      type: 'GET',
      url,
    }).done((resp) => {
      if (!resp) {
        $target.removeClass('disabled');
        return;
      }
      $target.parent().replaceWith($(resp).find('#diff-file-boxes .diff-file-body .file-body').children());
      onShowMoreFiles();
    }).fail(() => {
      $target.removeClass('disabled');
    });
  });
}
