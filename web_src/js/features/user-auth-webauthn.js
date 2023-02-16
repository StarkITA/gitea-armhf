import $ from 'jquery';
import {encode, decode} from 'uint8-to-base64';

const {appSubUrl, csrfToken} = window.config;

export function initUserAuthWebAuthn() {
  if ($('.user.signin.webauthn-prompt').length === 0) {
    return;
  }

  if (!detectWebAuthnSupport()) {
    return;
  }

  $.getJSON(`${appSubUrl}/user/webauthn/assertion`, {})
    .done((makeAssertionOptions) => {
      makeAssertionOptions.publicKey.challenge = decodeURLEncodedBase64(makeAssertionOptions.publicKey.challenge);
      for (let i = 0; i < makeAssertionOptions.publicKey.allowCredentials.length; i++) {
        makeAssertionOptions.publicKey.allowCredentials[i].id = decodeURLEncodedBase64(makeAssertionOptions.publicKey.allowCredentials[i].id);
      }
      navigator.credentials.get({
        publicKey: makeAssertionOptions.publicKey
      })
        .then((credential) => {
          verifyAssertion(credential);
        }).catch((err) => {
          // Try again... without the appid
          if (makeAssertionOptions.publicKey.extensions && makeAssertionOptions.publicKey.extensions.appid) {
            delete makeAssertionOptions.publicKey.extensions['appid'];
            navigator.credentials.get({
              publicKey: makeAssertionOptions.publicKey
            })
              .then((credential) => {
                verifyAssertion(credential);
              }).catch((err) => {
                webAuthnError('general', err.message);
              });
            return;
          }
          webAuthnError('general', err.message);
        });
    }).fail(() => {
      webAuthnError('unknown');
    });
}

function verifyAssertion(assertedCredential) {
  // Move data into Arrays incase it is super long
  const authData = new Uint8Array(assertedCredential.response.authenticatorData);
  const clientDataJSON = new Uint8Array(assertedCredential.response.clientDataJSON);
  const rawId = new Uint8Array(assertedCredential.rawId);
  const sig = new Uint8Array(assertedCredential.response.signature);
  const userHandle = new Uint8Array(assertedCredential.response.userHandle);
  $.ajax({
    url: `${appSubUrl}/user/webauthn/assertion`,
    type: 'POST',
    data: JSON.stringify({
      id: assertedCredential.id,
      rawId: encodeURLEncodedBase64(rawId),
      type: assertedCredential.type,
      clientExtensionResults: assertedCredential.getClientExtensionResults(),
      response: {
        authenticatorData: encodeURLEncodedBase64(authData),
        clientDataJSON: encodeURLEncodedBase64(clientDataJSON),
        signature: encodeURLEncodedBase64(sig),
        userHandle: encodeURLEncodedBase64(userHandle),
      },
    }),
    contentType: 'application/json; charset=utf-8',
    dataType: 'json',
    success: (resp) => {
      if (resp && resp['redirect']) {
        window.location.href = resp['redirect'];
      } else {
        window.location.href = '/';
      }
    },
    error: (xhr) => {
      if (xhr.status === 500) {
        webAuthnError('unknown');
        return;
      }
      webAuthnError('unable-to-process');
    }
  });
}

// Encode an ArrayBuffer into a URLEncoded base64 string.
function encodeURLEncodedBase64(value) {
  return encode(value)
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '');
}

// Dccode a URLEncoded base64 to an ArrayBuffer string.
function decodeURLEncodedBase64(value) {
  return decode(value
    .replace(/_/g, '/')
    .replace(/-/g, '+'));
}

function webauthnRegistered(newCredential) {
  const attestationObject = new Uint8Array(newCredential.response.attestationObject);
  const clientDataJSON = new Uint8Array(newCredential.response.clientDataJSON);
  const rawId = new Uint8Array(newCredential.rawId);

  return $.ajax({
    url: `${appSubUrl}/user/settings/security/webauthn/register`,
    type: 'POST',
    headers: {'X-Csrf-Token': csrfToken},
    data: JSON.stringify({
      id: newCredential.id,
      rawId: encodeURLEncodedBase64(rawId),
      type: newCredential.type,
      response: {
        attestationObject: encodeURLEncodedBase64(attestationObject),
        clientDataJSON: encodeURLEncodedBase64(clientDataJSON),
      },
    }),
    dataType: 'json',
    contentType: 'application/json; charset=utf-8',
  }).then(() => {
    window.location.reload();
  }).fail((xhr) => {
    if (xhr.status === 409) {
      webAuthnError('duplicated');
      return;
    }
    webAuthnError('unknown');
  });
}

function webAuthnError(errorType, message) {
  $('#webauthn-error [data-webauthn-error-msg]').hide();
  const $errorGeneral = $(`#webauthn-error [data-webauthn-error-msg=general]`);
  if (errorType === 'general') {
    $errorGeneral.show().text(message || 'unknown error');
  } else {
    const $errorTyped = $(`#webauthn-error [data-webauthn-error-msg=${errorType}]`);
    if ($errorTyped.length) {
      $errorTyped.show();
    } else {
      $errorGeneral.show().text(`unknown error type: ${errorType}`);
    }
  }
  $('#webauthn-error').modal('show');
}

function detectWebAuthnSupport() {
  if (!window.isSecureContext) {
    $('#register-button').prop('disabled', true);
    $('#login-button').prop('disabled', true);
    webAuthnError('insecure');
    return false;
  }

  if (typeof window.PublicKeyCredential !== 'function') {
    $('#register-button').prop('disabled', true);
    $('#login-button').prop('disabled', true);
    webAuthnError('browser');
    return false;
  }

  return true;
}

export function initUserAuthWebAuthnRegister() {
  if ($('#register-webauthn').length === 0) {
    return;
  }

  $('#webauthn-error').modal({allowMultiple: false});
  $('#register-webauthn').on('click', (e) => {
    e.preventDefault();
    if (!detectWebAuthnSupport()) {
      return;
    }
    webAuthnRegisterRequest();
  });
}

function webAuthnRegisterRequest() {
  if ($('#nickname').val() === '') {
    webAuthnError('empty');
    return;
  }
  $.post(`${appSubUrl}/user/settings/security/webauthn/request_register`, {
    _csrf: csrfToken,
    name: $('#nickname').val(),
  }).done((makeCredentialOptions) => {
    $('#nickname').closest('div.field').removeClass('error');

    makeCredentialOptions.publicKey.challenge = decodeURLEncodedBase64(makeCredentialOptions.publicKey.challenge);
    makeCredentialOptions.publicKey.user.id = decodeURLEncodedBase64(makeCredentialOptions.publicKey.user.id);
    if (makeCredentialOptions.publicKey.excludeCredentials) {
      for (let i = 0; i < makeCredentialOptions.publicKey.excludeCredentials.length; i++) {
        makeCredentialOptions.publicKey.excludeCredentials[i].id = decodeURLEncodedBase64(makeCredentialOptions.publicKey.excludeCredentials[i].id);
      }
    }

    navigator.credentials.create({
      publicKey: makeCredentialOptions.publicKey
    }).then(webauthnRegistered)
      .catch((err) => {
        if (!err) {
          webAuthnError('unknown');
          return;
        }
        webAuthnError('general', err.message);
      });
  }).fail((xhr) => {
    if (xhr.status === 409) {
      webAuthnError('duplicated');
      return;
    }
    webAuthnError('unknown');
  });
}
