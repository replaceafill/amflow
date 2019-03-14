'use strict';

import $ from 'jquery';
import panzoom from 'panzoom';
import * as client from './client';

function load() {
    // let c = client('http', 'localhost:2323');
    client.showWorkflow().then(function(resp) {
        let $area = $('.viewport')
        $area.empty();
        $area.append(resp.data.firstElementChild)
        panzoom($area.get(0), {
            minZoom: 0.01,
            zoomSpeed: 0.3,
            autocenter: true
        }).zoomAbs(0, 0, 0.1);

        $(".node", $(document)).on("click", function(event) {
            $(this).toggleClass("active");
        });
    });
}

function setupReloadButton() {
    document.querySelector('.reload-btn').addEventListener('click', function() {
        load();
    });
}

// case insensitive string match
$.expr[":"].iexact = $.expr.createPseudo(function(arg) {
    return function ( elem ) {
        return $(elem).text().toUpperCase() === arg.toUpperCase();
    };
});

// case insensitive substring match
$.expr[":"].icontains = $.expr.createPseudo(function(arg) {
    return function( elem ) {
        return $(elem).text().toUpperCase().indexOf(arg.toUpperCase()) >= 0;
    };
});

function search() {
    var search = $('#search').val() || '',
        full_match = $('#search-full-match').is(':checked'),
        only_titles = $('#search-only-titles').is(':checked'),
        string_matcher,
        selector_parts,
        selector;
    if (search) {
        // deselect all nodes
        $('g.node').removeClass('active');
        // select case insensitive string matcher
        string_matcher = full_match ? 'iexact' : 'icontains';
        // build selector
        selector_parts = ['g.node text'];
        if (only_titles) {
            // look only at the 5th text node child
            selector_parts.push(':nth-child(5)');
        }
        // e.g.: iexact("query")
        // e.g.: icontains("query")
        // XXX: sanitize/escape search query
        selector_parts.push(':' + string_matcher + '("' + search + '")');
        // build the selector and activate parent nodes
        $(selector_parts.join('')).parent().addClass('active');
    }
}

function setupSearch() {
    document.querySelector('.search-btn').addEventListener('click', search);
    document.querySelector('#search').addEventListener('keypress', function(e) {
        if (e.keyCode == 13) {
            search();
        }
    });
}

$(document).ready(function() {
    load();
    setupReloadButton();
    setupSearch();
});
